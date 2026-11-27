package featuremanagement

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// Re-exported SDK targeting types. FeatureTargeting prints as the API's own
// read model under -o json|yaml, so `get` shows the rules unchanged, full ids
// included; the table view is built from TargetingRows instead.
type (
	FeatureTargeting    = sdkfm.FeatureTargetingDetails
	Targeting           = sdkfm.Targeting
	TargetingRule       = sdkfm.TargetingRule
	TargetingCondition  = sdkfm.TargetingCondition
	VariantDistribution = sdkfm.VariantDistribution
	Rollout             = sdkfm.Rollout
	RolloutStep         = sdkfm.RolloutStep
)

// operatorSyntax maps each API operator to its --condition spelling (see the
// targeting write commands). An operator not listed prints as the API names
// it, so a newer server's operator still reads unambiguously.
var operatorSyntax = map[string]string{
	"IS":                    "=",
	"IS_NOT":                "!=",
	"GREATER_THAN":          ">",
	"GREATER_THAN_OR_EQUAL": ">=",
	"LESS_THAN":             "<",
	"LESS_THAN_OR_EQUAL":    "<=",
	"CONTAINS":              "contains",
	"DOES_NOT_CONTAIN":      "!contains",
	"STARTS_WITH":           "starts-with",
	"ENDS_WITH":             "ends-with",
	"IS_EMPTY":              "empty",
	"IS_NOT_EMPTY":          "!empty",
	"MATCHES_MAJOR":         "^",
	"MATCHES_MINOR":         "~",
	"BEFORE":                "before",
	"AFTER":                 "after",
	"BETWEEN":               "between",
	"EXISTS":                "exists",
	"DOES_NOT_EXIST":        "!exists",
}

// AllRequests is how a rule with no conditions reads: it matches every
// request.
const AllRequests = "all requests"

// FormatCondition renders a condition in --condition syntax, with the context key
// rather than a display name, so what a reader sees is what they can type:
// `targetingKey = user-1,user-2`, `signup before 2026-11-03T05:00Z`.
func FormatCondition(c TargetingCondition) string {
	op, ok := operatorSyntax[c.Operator]
	if !ok {
		op = c.Operator
	}
	s := c.ContextKey + " " + op
	if len(c.Values) == 0 {
		return s
	}
	vals := make([]string, len(c.Values))
	for i, v := range c.Values {
		vals[i] = formatConditionValue(c.ValueType, v)
	}
	return s + " " + strings.Join(vals, ",")
}

// FormatConditions renders a rule's conditions one per string, every one
// after the first prefixed "and " as the UI joins them; a rule with none
// reads "all requests".
func FormatConditions(cs []TargetingCondition) []string {
	if len(cs) == 0 {
		return []string{AllRequests}
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = FormatCondition(c)
		if i > 0 {
			out[i] = "and " + out[i]
		}
	}
	return out
}

func formatConditionValue(t sdkfm.ConditionValueType, v any) string {
	if t == sdkfm.ConditionValueTypeDateTime {
		var secs int64
		var err error
		switch x := v.(type) {
		case string:
			secs, err = strconv.ParseInt(x, 10, 64)
		case float64:
			secs = int64(x)
		default:
			err = fmt.Errorf("not epoch seconds")
		}
		if err == nil {
			return formatInstant(time.Unix(secs, 0))
		}
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// formatInstant prints a UTC instant to the minute when it falls on one, the
// precision rollout and date conditions are written in.
func formatInstant(t time.Time) string {
	t = t.UTC()
	if t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04Z")
	}
	return t.Format(time.RFC3339)
}

// FormatInstantSpaced is formatInstant with a space instead of the T, for the
// describe view's rollout schedule.
func FormatInstantSpaced(t time.Time) string {
	return strings.Replace(formatInstant(t), "T", " ", 1)
}

// FormatServe renders what a rule serves in --variant syntax: one variant
// key for a single variant at 100%, otherwise key=percentage pairs.
func FormatServe(serve []VariantDistribution) string {
	if len(serve) == 0 {
		return "-"
	}
	if len(serve) == 1 && serve[0].Percentage == 100 {
		return serve[0].VariantKey
	}
	parts := make([]string, len(serve))
	for i, d := range serve {
		parts[i] = d.VariantKey + "=" + formatPercent(d.Percentage)
	}
	return strings.Join(parts, ",")
}

// FormatRolloutSummary renders a rollout on one line, `10% -> 50% (linear)
// -> 100%`, marking each step reached linearly; "-" when the rule has none.
func FormatRolloutSummary(r *Rollout) string {
	if r == nil || len(r.Steps) == 0 {
		return "-"
	}
	parts := make([]string, len(r.Steps))
	for i, s := range r.Steps {
		parts[i] = formatPercent(s.Percentage) + "%" + transitionSuffix(s.Transition)
	}
	return strings.Join(parts, " -> ")
}

// FormatRolloutStep renders one step for the describe view's schedule.
func FormatRolloutStep(s RolloutStep) string {
	return formatPercent(s.Percentage) + "% at " + FormatInstantSpaced(s.EffectiveTime) + transitionSuffix(s.Transition)
}

func transitionSuffix(t sdkfm.RolloutTransition) string {
	switch t {
	case "", sdkfm.RolloutTransitionStep:
		return ""
	default:
		return " (" + strings.ToLower(string(t)) + ")"
	}
}

func formatPercent(p float64) string { return strconv.FormatFloat(p, 'f', -1, 64) }

// ShortRuleID is the prefix of a rule id the table shows, like a short git
// SHA.
func ShortRuleID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	if id == "" {
		return "-"
	}
	return id
}

// ShadowedRules returns a warning for every rule placed below a rule with no
// conditions and no rollout, which matches all requests, so the lower rule is
// never evaluated. A rollout is part of the match: the service compiles it to
// a condition, so requests outside its percentage, before its first step, or
// without the bucketing attribute fall through to the next rule. Positions
// are 0-based, as dtctl takes them.
func ShadowedRules(rules []TargetingRule) []string {
	var out []string
	for i, r := range rules {
		if len(r.Conditions) == 0 && r.Rollout == nil {
			for j := i + 1; j < len(rules); j++ {
				out = append(out, fmt.Sprintf("rule %d will not be evaluated: rule %d matches all requests", j, i))
			}
			break
		}
	}
	return out
}

// TargetingRow is one CSV row of `get fm feature-targeting`: one rule of
// one stage, or the stage alone when it has no rules. Table and wide output
// print TargetingStageRows and then TargetingRuleRows per stage instead.
type TargetingRow struct {
	Stage      string `table:"STAGE"`
	Targeting  string `table:"TARGETING"`
	Position   string `table:"#"`
	ID         string `table:"ID"`
	Name       string `table:"NAME,wide"`
	Conditions string `table:"CONDITIONS"`
	Variant    string `table:"VARIANT"`
	Rollout    string `table:"ROLLOUT"`
	Bucketing  string `table:"BUCKETING,wide"`
	Version    int    `table:"VERSION,wide"`
}

// TargetingRows flattens per-stage targeting into table rows, keeping the
// stages in the order given and each stage's rules in evaluation order.
func TargetingRows(items []FeatureTargeting) []TargetingRow {
	var rows []TargetingRow
	for _, ft := range items {
		state := onOff(ft.Enabled)
		if len(ft.Targeting.Rules) == 0 {
			rows = append(rows, TargetingRow{
				Stage: ft.ReleaseStageKey, Targeting: state, Position: "-", ID: "-", Name: "",
				Conditions: "(no rules)", Variant: "-", Rollout: "-", Bucketing: "", Version: ft.Version,
			})
			continue
		}
		for i, r := range ft.Targeting.Rules {
			rows = append(rows, TargetingRow{
				Stage:      ft.ReleaseStageKey,
				Targeting:  state,
				Position:   strconv.Itoa(i),
				ID:         ShortRuleID(r.ID),
				Name:       r.Name,
				Conditions: strings.Join(FormatConditions(r.Conditions), " "),
				Variant:    FormatServe(r.Serve),
				Rollout:    FormatRolloutSummary(r.Rollout),
				Bucketing:  r.BucketingKey,
				Version:    ft.Version,
			})
		}
	}
	return rows
}

// TargetingStageRow is one row of the stage summary that `get fm
// feature-targeting` prints first: whether targeting is on and how many
// rules the stage has.
type TargetingStageRow struct {
	Stage     string `table:"STAGE"`
	Targeting string `table:"TARGETING"`
	Rules     int    `table:"RULES"`
	Version   int    `table:"VERSION,wide"`
}

// TargetingRuleRow is one rule in a stage's rule table. Conditions holds one
// condition per line, the way the describe view shows them.
type TargetingRuleRow struct {
	Position   string `table:"#"`
	ID         string `table:"ID"`
	Name       string `table:"NAME,wide"`
	Conditions string `table:"CONDITIONS"`
	Variant    string `table:"VARIANT"`
	Rollout    string `table:"ROLLOUT"`
	Bucketing  string `table:"BUCKETING,wide"`
}

// TargetingStageRows summarizes per-stage targeting, one row per stage in
// the order given.
func TargetingStageRows(items []FeatureTargeting) []TargetingStageRow {
	rows := make([]TargetingStageRow, 0, len(items))
	for _, ft := range items {
		rows = append(rows, TargetingStageRow{
			Stage:     ft.ReleaseStageKey,
			Targeting: onOff(ft.Enabled),
			Rules:     len(ft.Targeting.Rules),
			Version:   ft.Version,
		})
	}
	return rows
}

// TargetingRuleRows returns one stage's rules in evaluation order.
func TargetingRuleRows(ft FeatureTargeting) []TargetingRuleRow {
	rows := make([]TargetingRuleRow, 0, len(ft.Targeting.Rules))
	for i, r := range ft.Targeting.Rules {
		rows = append(rows, TargetingRuleRow{
			Position:   strconv.Itoa(i),
			ID:         ShortRuleID(r.ID),
			Name:       r.Name,
			Conditions: strings.Join(FormatConditions(r.Conditions), "\n"),
			Variant:    FormatServe(r.Serve),
			Rollout:    FormatRolloutSummary(r.Rollout),
			Bucketing:  r.BucketingKey,
		})
	}
	return rows
}

// ListFeatureTargeting returns a feature's targeting in every release stage,
// in the project's stage order. The feature is a key or, when no feature has
// that key, a name.
//
// The stage order comes from the release stages' display order, which needs
// the release-stages read scope; without it the stages are listed by key, so
// the command still works with the targeting scope alone.
func (h *Handler) ListFeatureTargeting(projectKey, featureKeyOrName string) ([]FeatureTargeting, error) {
	list, err := withFeatureKey(h, projectKey, featureKeyOrName, func(featureKey string) (*sdkfm.FeatureTargetingList, error) {
		return h.sdk.ListFeatureTargeting(context.Background(), projectKey, featureKey, sdkfm.ListOptions{})
	})
	if err != nil {
		return nil, err
	}
	items := list.Items
	order := map[string]int{}
	if stages, err := h.ListReleaseStages(projectKey); err == nil {
		for i, s := range stages {
			order[s.Key] = i
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, aok := order[items[i].ReleaseStageKey]
		b, bok := order[items[j].ReleaseStageKey]
		if aok != bok {
			return aok
		}
		if aok && a != b {
			return a < b
		}
		return items[i].ReleaseStageKey < items[j].ReleaseStageKey
	})
	return items, nil
}

// GetFeatureTargeting returns a feature's targeting in one release stage. The
// feature is a key or a name; the stage is always a key, as on the write
// commands, where it is what approval policy matches.
func (h *Handler) GetFeatureTargeting(projectKey, featureKeyOrName, releaseStageKey string) (*FeatureTargeting, error) {
	return withFeatureKey(h, projectKey, featureKeyOrName, func(featureKey string) (*sdkfm.FeatureTargetingDetails, error) {
		return h.sdk.GetFeatureTargeting(context.Background(), projectKey, featureKey, releaseStageKey)
	})
}

// withFeatureKey calls fn with the feature argument as a key and, when the
// server says no feature has that key, resolves it as a name (which needs
// the features read scope) and calls fn again with the key it names. A name
// nobody has fails with the feature lookup's "not found" error.
func withFeatureKey[T any](h *Handler, projectKey, keyOrName string, fn func(string) (T, error)) (T, error) {
	return withResolvedKey(keyOrName, fn, func() (string, error) {
		f, err := h.GetFeature(projectKey, keyOrName)
		if err != nil {
			return "", err
		}
		return f.Key, nil
	})
}

// withReleaseStageKey is withFeatureKey for a release stage argument.
func withReleaseStageKey[T any](h *Handler, projectKey, keyOrName string, fn func(string) (T, error)) (T, error) {
	return withResolvedKey(keyOrName, fn, func() (string, error) {
		rs, err := h.GetReleaseStage(projectKey, keyOrName)
		if err != nil {
			return "", err
		}
		return rs.Key, nil
	})
}

// withResolvedKey calls fn with keyOrName as a key and, on a key miss, with
// the key resolve finds for it as a name. fn's own error stands when resolve
// finds the same key; resolve's error stands when it finds none.
func withResolvedKey[T any](keyOrName string, fn func(string) (T, error), resolve func() (string, error)) (T, error) {
	v, err := fn(keyOrName)
	if err == nil || !isKeyMiss(err) {
		return v, err
	}
	key, rerr := resolve()
	if rerr != nil {
		return v, rerr
	}
	if key == keyOrName {
		return v, err
	}
	return fn(key)
}
