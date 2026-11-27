package featuremanagement

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// targetingKeyContextKey is the context key every evaluation carries. It is
// always a STRING, whether or not the project declares it as a context
// attribute.
const targetingKeyContextKey = "targetingKey"

// conditionOperators maps each --condition operator to the API's name. It is
// operatorSyntax read the other way.
var conditionOperators = func() map[string]string {
	m := make(map[string]string, len(operatorSyntax))
	for api, syntax := range operatorSyntax {
		m[syntax] = api
	}
	return m
}()

// operatorsByType lists the operators each value type supports, in --condition
// syntax. EXISTS and DOES_NOT_EXIST work on every type.
var operatorsByType = map[sdkfm.ConditionValueType][]string{
	sdkfm.ConditionValueTypeString:   {"=", "!=", "contains", "!contains", "starts-with", "ends-with", "empty", "!empty"},
	sdkfm.ConditionValueTypeNumber:   {"=", "!=", ">", ">=", "<", "<="},
	sdkfm.ConditionValueTypeSemver:   {"=", "!=", ">", ">=", "<", "<=", "^", "~"},
	sdkfm.ConditionValueTypeBoolean:  {"=", "!="},
	sdkfm.ConditionValueTypeDateTime: {"before", "after", "between"},
}

var anyTypeOperators = []string{"exists", "!exists"}

// valueCount is how many values an operator takes: -1 for one or more.
func valueCount(op string) int {
	switch op {
	case "empty", "!empty", "exists", "!exists":
		return 0
	case "between":
		return 2
	case ">", ">=", "<", "<=", "^", "~", "before", "after":
		return 1
	default:
		return -1
	}
}

// IsListOperator reports whether an operator takes a list of values, which
// --condition-add and --condition-remove can change one value at a time.
func IsListOperator(op string) bool { return valueCount(op) == -1 }

// ConditionExpr is a parsed --condition expression, before its values are
// typed.
type ConditionExpr struct {
	ContextKey string
	Operator   string // --condition syntax
	Values     []string
}

// ParseConditionExpr parses '<context-key> <operator> <values>', values
// comma-separated. It checks the operator and the number of values; the value
// type comes later, from the project's context attributes.
func ParseConditionExpr(s string) (ConditionExpr, error) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return ConditionExpr{}, fmt.Errorf("invalid condition %q: want '<context-key> <operator> <values>'", s)
	}
	e := ConditionExpr{ContextKey: fields[0], Operator: fields[1]}
	if _, ok := conditionOperators[e.Operator]; !ok {
		return ConditionExpr{}, fmt.Errorf("invalid condition %q: unknown operator %q (one of %s)", s, e.Operator, strings.Join(sortedOperators(), " "))
	}
	if rest := strings.TrimSpace(strings.Join(fields[2:], " ")); rest != "" {
		for _, v := range strings.Split(rest, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				return ConditionExpr{}, fmt.Errorf("invalid condition %q: empty value", s)
			}
			e.Values = append(e.Values, v)
		}
	}
	switch n := valueCount(e.Operator); {
	case n == 0 && len(e.Values) > 0:
		return ConditionExpr{}, fmt.Errorf("invalid condition %q: %s takes no values", s, e.Operator)
	case n == -1 && len(e.Values) == 0:
		return ConditionExpr{}, fmt.Errorf("invalid condition %q: %s needs at least one value", s, e.Operator)
	case n > 0 && len(e.Values) != n:
		return ConditionExpr{}, fmt.Errorf("invalid condition %q: %s takes %d value(s), got %d", s, e.Operator, n, len(e.Values))
	}
	return e, nil
}

func sortedOperators() []string {
	ops := make([]string, 0, len(conditionOperators))
	for op := range conditionOperators {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// ContextKeyTypes maps a project's context keys to their value types.
type ContextKeyTypes map[string]sdkfm.ConditionValueType

// NewContextKeyTypes builds the map from a project's context attributes.
func NewContextKeyTypes(attrs []ContextAttribute) ContextKeyTypes {
	m := ContextKeyTypes{}
	for _, a := range attrs {
		m[a.ContextKey] = sdkfm.ConditionValueType(a.Type)
	}
	return m
}

// typeOf returns a context key's value type, or an error naming the fix when
// the project declares no such key.
func (t ContextKeyTypes) typeOf(projectKey, contextKey string) (sdkfm.ConditionValueType, error) {
	if vt, ok := t[contextKey]; ok {
		return vt, nil
	}
	if contextKey == targetingKeyContextKey {
		return sdkfm.ConditionValueTypeString, nil
	}
	return "", fmt.Errorf("context key %q is not a context attribute of project %q; list them with 'dtctl get fm context-attributes' or create one with 'dtctl create fm context-attribute'", contextKey, projectKey)
}

// TypedCondition turns a parsed expression into an API condition: it takes
// the value type from the context attribute, checks that the type supports
// the operator, and converts each value. now resolves relative DATE_TIME
// values.
func (t ContextKeyTypes) TypedCondition(projectKey string, e ConditionExpr, now time.Time) (TargetingCondition, error) {
	vt, err := t.typeOf(projectKey, e.ContextKey)
	if err != nil {
		return TargetingCondition{}, err
	}
	if !operatorSupported(vt, e.Operator) {
		return TargetingCondition{}, fmt.Errorf("operator %s does not apply to %s (a %s): use one of %s",
			e.Operator, e.ContextKey, vt, strings.Join(append(append([]string{}, operatorsByType[vt]...), anyTypeOperators...), " "))
	}
	c := TargetingCondition{ValueType: vt, ContextKey: e.ContextKey, Operator: conditionOperators[e.Operator]}
	for _, raw := range e.Values {
		v, err := typedValue(vt, raw, now)
		if err != nil {
			return TargetingCondition{}, fmt.Errorf("condition on %s: %w", e.ContextKey, err)
		}
		c.Values = append(c.Values, v)
	}
	return c, nil
}

func operatorSupported(vt sdkfm.ConditionValueType, op string) bool {
	for _, o := range anyTypeOperators {
		if o == op {
			return true
		}
	}
	ops, known := operatorsByType[vt]
	if !known {
		// A value type this dtctl doesn't know: let the server decide.
		return true
	}
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}

func typedValue(vt sdkfm.ConditionValueType, raw string, now time.Time) (any, error) {
	switch vt {
	case sdkfm.ConditionValueTypeNumber:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("%q is not a number", raw)
		}
		return f, nil
	case sdkfm.ConditionValueTypeBoolean:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", raw)
		}
		return b, nil
	case sdkfm.ConditionValueTypeDateTime:
		t, err := ParseTimeSpec(raw, now)
		if err != nil {
			return nil, err
		}
		return strconv.FormatInt(t.Unix(), 10), nil
	default:
		return raw, nil
	}
}

// conditionValueKey is how two condition values compare: by their printed
// form, so a JSON number read back equals the float a flag parsed.
func conditionValueKey(vt sdkfm.ConditionValueType, v any) string {
	return formatConditionValue(vt, v)
}

var offsetPattern = regexp.MustCompile(`^\+([0-9]+)([smhd])$`)

// timestampLayouts are the zoned forms a rollout or DATE_TIME value accepts:
// minutes, seconds, or fractional seconds.
var timestampLayouts = []string{"2006-01-02T15:04Z07:00", time.RFC3339, time.RFC3339Nano}

// zonelessLayouts are recognized only to explain that a zone is required.
var zonelessLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"}

// ParseTimeSpec reads a point in time: a timestamp with an explicit zone,
// Unix seconds, `now`, or an offset from now (+30m, +2h, +1d; a day is 24
// hours). A timestamp without a zone is an error rather than being read in
// the machine's local time, and 13 or more digits are taken to be
// milliseconds by mistake.
func ParseTimeSpec(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return now.UTC(), nil
	}
	if m := offsetPattern.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid offset %q", s)
		}
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		return now.UTC().Add(time.Duration(n) * unit), nil
	}
	if isDigits(s) {
		if len(s) >= 13 {
			return time.Time{}, fmt.Errorf("time %q looks like milliseconds; divide by 1000 for Unix seconds", s)
		}
		secs, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid Unix seconds %q", s)
		}
		return time.Unix(secs, 0).UTC(), nil
	}
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	for _, layout := range zonelessLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return time.Time{}, fmt.Errorf("time %q has no zone: add Z for UTC (%sZ) or an offset like +01:00", s, s)
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q: want a timestamp with a zone (2026-12-01T09:00Z), Unix seconds, now, or an offset like +30m, +2h, +1d", s)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ParseServe reads --variant: one variant served at 100%, or a split like
// baseline=50,variation-a=50 that totals 100. Every key must be one of the
// feature's variants.
func ParseServe(s string, variants []Variant) ([]VariantDistribution, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("--variant must not be empty")
	}
	known := map[string]bool{}
	keys := make([]string, 0, len(variants))
	for _, v := range variants {
		known[v.Key] = true
		keys = append(keys, v.Key)
	}
	check := func(k string) error {
		if len(variants) > 0 && !known[k] {
			return fmt.Errorf("unknown variant %q: the feature's variants are %s", k, strings.Join(keys, ", "))
		}
		return nil
	}
	if !strings.Contains(s, "=") {
		if err := check(s); err != nil {
			return nil, err
		}
		return []VariantDistribution{{VariantKey: s, Percentage: 100}}, nil
	}
	var out []VariantDistribution
	seen := map[string]bool{}
	total := 0.0
	for _, part := range strings.Split(s, ",") {
		k, pct, ok := strings.Cut(strings.TrimSpace(part), "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --variant %q: want <variant> or <variant>=<percent>,...", s)
		}
		if seen[k] {
			return nil, fmt.Errorf("invalid --variant %q: variant %q appears twice", s, k)
		}
		seen[k] = true
		if err := check(k); err != nil {
			return nil, err
		}
		p, err := parsePercent(pct)
		if err != nil {
			return nil, fmt.Errorf("invalid --variant %q: %w", s, err)
		}
		total += p
		out = append(out, VariantDistribution{VariantKey: k, Percentage: p})
	}
	if math.Abs(total-100) > 1e-9 {
		return nil, fmt.Errorf("invalid --variant %q: the split totals %s%%, not 100%%", s, formatPercent(total))
	}
	return out, nil
}

func parsePercent(s string) (float64, error) {
	p, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	if err != nil || math.IsNaN(p) || p < 0 || p > 100 {
		return 0, fmt.Errorf("percentage %q must be a number from 0 to 100", s)
	}
	return p, nil
}

// ParseRollout reads --rollout steps, each <time>=<percent>[,linear]. now
// resolves `now` and offsets, once for the whole invocation. Steps must be in
// time order.
func ParseRollout(specs []string, now time.Time) (*Rollout, error) {
	r := &Rollout{}
	for _, spec := range specs {
		when, rest, ok := strings.Cut(strings.TrimSpace(spec), "=")
		if !ok {
			return nil, fmt.Errorf("invalid --rollout %q: want <time>=<percent>[,linear]", spec)
		}
		pct, transition, hasTransition := strings.Cut(rest, ",")
		t, err := ParseTimeSpec(when, now)
		if err != nil {
			return nil, fmt.Errorf("invalid --rollout %q: %w", spec, err)
		}
		p, err := parsePercent(pct)
		if err != nil {
			return nil, fmt.Errorf("invalid --rollout %q: %w", spec, err)
		}
		step := RolloutStep{EffectiveTime: t, Percentage: p}
		if hasTransition {
			switch strings.ToLower(strings.TrimSpace(transition)) {
			case "linear":
				step.Transition = sdkfm.RolloutTransitionLinear
			case "step":
			default:
				return nil, fmt.Errorf("invalid --rollout %q: transition %q must be linear or step", spec, transition)
			}
		}
		if n := len(r.Steps); n > 0 && !t.After(r.Steps[n-1].EffectiveTime) {
			return nil, fmt.Errorf("invalid --rollout %q: steps must be in time order, each after the one before", spec)
		}
		r.Steps = append(r.Steps, step)
	}
	return r, nil
}

// ResolveRulePosition finds a rule by --position: a 0-based position or a
// rule id, full or a unique prefix. insert allows the position one past the
// last rule, where create appends.
func ResolveRulePosition(rules []TargetingRule, s string, insert bool) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("--position must not be empty")
	}
	if isDigits(s) {
		n, err := strconv.Atoi(s)
		limit := len(rules) - 1
		if insert {
			limit = len(rules)
		}
		if err != nil || n > limit {
			switch {
			case len(rules) == 0 && !insert:
				return 0, fmt.Errorf("position %s is out of range: the stage has no rules", s)
			default:
				return 0, fmt.Errorf("position %s is out of range: positions are 0 to %d", s, limit)
			}
		}
		return n, nil
	}
	var matches []int
	lower := strings.ToLower(s)
	for i, r := range rules {
		if strings.HasPrefix(strings.ToLower(r.ID), lower) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("no rule has id %q (pass a 0-based position or a rule id from 'dtctl get fm feature-targeting')", s)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = rules[m].ID
		}
		return 0, fmt.Errorf("rule id prefix %q is ambiguous: it matches %s", s, strings.Join(ids, ", "))
	}
}
