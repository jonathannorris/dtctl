package featuremanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// Re-exported SDK feature write types.
type (
	FeatureCreate     = sdkfm.FeatureCreate
	FeatureFlagCreate = sdkfm.FeatureFlagCreate
	InitialFlagCreate = sdkfm.InitialFlagCreate
)

// DefaultVariants are the variants a feature created without --variants
// gets, the UI's default: off first, on last.
var DefaultVariants = []string{"disabled", "enabled"}

// FeatureFile reads a feature from a create file: the API's FeatureCreate
// fields, or a feature exported with get -o yaml, whose read-only members
// are dropped.
func FeatureFile(projectKey string) ResourceFile {
	return ResourceFile{
		Kind:     "fm/feature",
		ReadOnly: []string{"modificationInfo", "resourceContext", "version", "status"},
		Parents:  map[string]string{"projectKey": projectKey},
	}
}

// FeatureCreateSpec is a feature created from flags.
type FeatureCreateSpec struct {
	Key, Name, Description, Owner string
	Type                          FeatureType
	Maintainers                   []string
	// Variants are the variant keys in display order (default
	// DefaultVariants). Each variant's name is its key.
	Variants []string
	// FlagKey and FlagType describe the one flag the feature is created
	// with (default: a BOOLEAN keyed like the feature).
	FlagKey  string
	FlagType FlagType
	// FlagValues are <variant>=<value> pairs: one per variant, or none for
	// the type's defaults.
	FlagValues []string
}

// Body builds the create request.
func (s FeatureCreateSpec) Body() (FeatureCreate, error) {
	keys := s.Variants
	if len(keys) == 0 {
		keys = DefaultVariants
	}
	body := FeatureCreate{
		Key: s.Key, Name: s.Name, Description: s.Description, Type: s.Type,
		Owner: s.Owner, Maintainers: s.Maintainers,
	}
	seen := map[string]bool{}
	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			return FeatureCreate{}, fmt.Errorf("--variants: variant keys must be unique and non-empty")
		}
		seen[k] = true
		order := i
		body.Variants = append(body.Variants, Variant{Key: k, Name: k, DisplayOrder: &order})
	}
	flag := &InitialFlagCreate{Key: s.FlagKey, Type: s.FlagType}
	if flag.Key == "" {
		flag.Key = s.Key
	}
	if flag.Type == "" {
		flag.Type = sdkfm.FlagTypeBoolean
	}
	if len(s.FlagValues) > 0 {
		values, err := ParseFlagValues(flag.Type, s.FlagValues, body.Variants)
		if err != nil {
			return FeatureCreate{}, err
		}
		flag.Variants = values
	}
	body.Flag = flag
	return body, nil
}

// ParseFlagValue reads a flag value given on the command line, by the
// flag's type: true|false, a number, a string as given, or a JSON value.
func ParseFlagValue(t FlagType, raw string) (any, error) {
	switch t {
	case sdkfm.FlagTypeBoolean:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", raw)
		}
		return b, nil
	case sdkfm.FlagTypeNumber:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", raw)
		}
		return f, nil
	case sdkfm.FlagTypeJSON:
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("%q is not valid JSON: %w", raw, err)
		}
		return v, nil
	default:
		return raw, nil
	}
}

// ParseFlagValues reads <variant>=<value> pairs for one flag. It needs one
// value per variant: the update endpoint rejects a flag missing one, and the
// create endpoint would store it with the variant unset.
func ParseFlagValues(t FlagType, pairs []string, variants []Variant) (map[string]any, error) {
	known := map[string]bool{}
	for _, v := range variants {
		known[v.Key] = true
	}
	values := map[string]any{}
	for _, pair := range pairs {
		k, raw, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid flag value %q: want <variant>=<value>", pair)
		}
		if !known[k] {
			return nil, fmt.Errorf("invalid flag value %q: the feature has no variant %q (variants: %s)", pair, k, variantKeys(variants))
		}
		if _, dup := values[k]; dup {
			return nil, fmt.Errorf("invalid flag value %q: variant %q has two values", pair, k)
		}
		v, err := ParseFlagValue(t, raw)
		if err != nil {
			return nil, fmt.Errorf("value for variant %q: %w", k, err)
		}
		values[k] = v
	}
	var missing []string
	for _, v := range variants {
		if _, ok := values[v.Key]; !ok {
			missing = append(missing, v.Key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("give a value for every variant, or none for the type's defaults; missing: %s", strings.Join(missing, ", "))
	}
	return values, nil
}

// DefaultFlagValues are the values the service gives a new flag on create:
// the variant shown last gets the type's "on" value, every other variant its
// "off" value. The update endpoint fills nothing, so dtctl fills these when a
// flag is added to an existing feature without values.
func DefaultFlagValues(t FlagType, variants []Variant) map[string]any {
	on, off := flagDefaults(t)
	last := -1
	for i, v := range variants {
		if last < 0 || order(v) >= order(variants[last]) {
			last = i
		}
	}
	values := map[string]any{}
	for i, v := range variants {
		if i == last {
			values[v.Key] = on
		} else {
			values[v.Key] = off
		}
	}
	return values
}

func flagDefaults(t FlagType) (on, off any) {
	switch t {
	case sdkfm.FlagTypeString:
		return "Enabled", "Disabled"
	case sdkfm.FlagTypeNumber:
		return float64(100), float64(0)
	case sdkfm.FlagTypeJSON:
		return map[string]any{"enabled": true}, map[string]any{"enabled": false}
	default:
		return true, false
	}
}

func order(v Variant) int {
	if v.DisplayOrder == nil {
		return 0
	}
	return *v.DisplayOrder
}

func variantKeys(vs []Variant) string {
	keys := make([]string, len(vs))
	for i, v := range vs {
		keys[i] = v.Key
	}
	return strings.Join(keys, ", ")
}

// FeatureWriteResult is the outcome of a feature write.
type FeatureWriteResult struct {
	// Feature is the feature after the write, or as read when nothing
	// changed.
	Feature *Feature
	// Changed is false when the feature already was what the command asked
	// for, so nothing was sent.
	Changed bool
	// Warnings are follow-ups the write leaves, such as targeting that still
	// names a removed variant.
	Warnings []string
}

// CreateFeature creates a feature.
func (h *Handler) CreateFeature(projectKey string, body FeatureCreate) (*FeatureWriteResult, error) {
	created, err := h.sdk.CreateFeature(context.Background(), projectKey, body)
	if err != nil {
		return nil, err
	}
	f := fromSDKFeature(created)
	return &FeatureWriteResult{Feature: &f, Changed: true}, nil
}

// readFeature reads a feature by key or name with every member the API
// returned, for a read-modify-write.
func (h *Handler) readFeature(projectKey, keyOrName string) (*sdkfm.FeatureDetails, error) {
	return withFeatureKey(h, projectKey, keyOrName, func(key string) (*sdkfm.FeatureDetails, error) {
		return h.sdk.GetFeature(context.Background(), projectKey, key)
	})
}

// featureAfterWrite reads the feature a write returned no body for.
func (h *Handler) featureAfterWrite(projectKey, key string) (*Feature, error) {
	d, err := h.sdk.GetFeature(context.Background(), projectKey, key)
	if err != nil {
		return nil, err
	}
	f := fromSDKFeature(d)
	return &f, nil
}

// SetFeatureStatus archives or reactivates a feature (the status patch). A
// feature already in that status is left alone.
func (h *Handler) SetFeatureStatus(projectKey, keyOrName string, status FeatureStatus, opts WriteOptions) (*FeatureWriteResult, error) {
	d, err := h.readFeature(projectKey, keyOrName)
	if err != nil {
		return nil, err
	}
	if d.Status == status {
		f := fromSDKFeature(d)
		return &FeatureWriteResult{Feature: &f}, nil
	}
	if _, err := h.sdk.PatchFeature(context.Background(), projectKey, d.Key, sdkfm.FeaturePatch{Status: status}, opts.version(d.Version)); err != nil {
		return nil, conflictError(err, fmt.Sprintf("feature %q", d.Key))
	}
	f, err := h.featureAfterWrite(projectKey, d.Key)
	if err != nil {
		return nil, err
	}
	return &FeatureWriteResult{Feature: f, Changed: true}, nil
}

// FeatureMetadataChange is a descriptive update; nil fields are kept.
type FeatureMetadataChange struct {
	Name, Description, Owner *string
	Type                     *FeatureType
	Maintainers              *[]string
}

// UpdateFeatureMetadata changes a feature's descriptive fields through the
// metadata endpoint (features.metadata:write), which cannot change behavior.
// It sends every descriptive field back, since the endpoint clears what a
// request leaves out.
func (h *Handler) UpdateFeatureMetadata(projectKey, keyOrName string, c FeatureMetadataChange, opts WriteOptions) (*FeatureWriteResult, error) {
	d, err := h.readFeature(projectKey, keyOrName)
	if err != nil {
		return nil, err
	}
	body := sdkfm.NewFeatureMetadataUpdate(d)
	before := body
	before.Maintainers = append([]string{}, body.Maintainers...)
	if c.Name != nil {
		body.Name = *c.Name
	}
	if c.Description != nil {
		body.Description = *c.Description
	}
	if c.Owner != nil {
		body.Owner = *c.Owner
	}
	if c.Type != nil {
		body.Type = *c.Type
	}
	if c.Maintainers != nil {
		body.Maintainers = append([]string{}, *c.Maintainers...)
	}
	if sameJSON(before, body) {
		f := fromSDKFeature(d)
		return &FeatureWriteResult{Feature: &f}, nil
	}
	if _, err := h.sdk.UpdateFeatureMetadata(context.Background(), projectKey, d.Key, body, opts.version(d.Version)); err != nil {
		return nil, conflictError(err, fmt.Sprintf("feature %q", d.Key))
	}
	f, err := h.featureAfterWrite(projectKey, d.Key)
	if err != nil {
		return nil, err
	}
	return &FeatureWriteResult{Feature: f, Changed: true}, nil
}

// VariantFlagChange adds or removes variants and flags through the full
// feature update. Several can be combined; each value is checked against the
// resulting variant and flag lists.
type VariantFlagChange struct {
	AddVariant   string
	VariantName  string
	DisplayOrder *int
	// Values are <flag-key>=<value>, each flag's value for AddVariant.
	Values        []string
	RemoveVariant string

	AddFlag         string
	FlagType        FlagType
	FlagName        string
	FlagDescription string
	// FlagValues are <variant>=<value>, AddFlag's value per variant; none
	// fills the type's defaults.
	FlagValues []string
	RemoveFlag string
}

// ChangeFeatureVariantsAndFlags applies a VariantFlagChange as one full
// feature update, sending back every member it read, flag schemas and
// fields this dtctl doesn't know included.
func (h *Handler) ChangeFeatureVariantsAndFlags(projectKey, keyOrName string, c VariantFlagChange, opts WriteOptions) (*FeatureWriteResult, error) {
	d, err := h.readFeature(projectKey, keyOrName)
	if err != nil {
		return nil, err
	}
	body := sdkfm.NewFeatureUpdate(d)

	if c.RemoveFlag != "" {
		i := flagIndex(body.Flags, c.RemoveFlag)
		if i < 0 {
			return nil, fmt.Errorf("feature %q has no flag %q", d.Key, c.RemoveFlag)
		}
		body.Flags = append(body.Flags[:i:i], body.Flags[i+1:]...)
	}
	if c.RemoveVariant != "" {
		i := variantIndex(body.Variants, c.RemoveVariant)
		if i < 0 {
			return nil, fmt.Errorf("feature %q has no variant %q (variants: %s)", d.Key, c.RemoveVariant, variantKeys(body.Variants))
		}
		body.Variants = append(body.Variants[:i:i], body.Variants[i+1:]...)
		for fi := range body.Flags {
			vals := map[string]any{}
			for k, v := range body.Flags[fi].Variants {
				if k != c.RemoveVariant {
					vals[k] = v
				}
			}
			body.Flags[fi].Variants = vals
		}
	}
	if c.AddVariant != "" {
		if variantIndex(body.Variants, c.AddVariant) >= 0 {
			return nil, fmt.Errorf("feature %q already has a variant %q", d.Key, c.AddVariant)
		}
		name := c.VariantName
		if name == "" {
			name = c.AddVariant
		}
		next := 0
		for _, v := range body.Variants {
			if o := order(v) + 1; o > next {
				next = o
			}
		}
		if c.DisplayOrder != nil {
			next = *c.DisplayOrder
		}
		body.Variants = append(body.Variants, Variant{Key: c.AddVariant, Name: name, DisplayOrder: &next})
		if err := addVariantValues(body.Flags, c.AddVariant, c.Values); err != nil {
			return nil, err
		}
	} else if len(c.Values) > 0 {
		return nil, fmt.Errorf("--value needs --add-variant")
	}
	if c.AddFlag != "" {
		if flagIndex(body.Flags, c.AddFlag) >= 0 {
			return nil, fmt.Errorf("feature %q already has a flag %q", d.Key, c.AddFlag)
		}
		t := c.FlagType
		if t == "" {
			t = sdkfm.FlagTypeBoolean
		}
		values := DefaultFlagValues(t, body.Variants)
		if len(c.FlagValues) > 0 {
			if values, err = ParseFlagValues(t, c.FlagValues, body.Variants); err != nil {
				return nil, err
			}
		}
		name := c.FlagName
		if name == "" {
			name = c.AddFlag
		}
		body.Flags = append(body.Flags, FeatureFlag{Key: c.AddFlag, Name: name, Description: c.FlagDescription, Type: t, Variants: values})
	} else if len(c.FlagValues) > 0 {
		return nil, fmt.Errorf("--flag-value needs --add-flag")
	}

	updated, err := h.sdk.UpdateFeature(context.Background(), projectKey, d.Key, body, opts.version(d.Version))
	if err != nil {
		return nil, conflictError(err, fmt.Sprintf("feature %q", d.Key))
	}
	if updated == nil || updated.Key == "" {
		if updated, err = h.sdk.GetFeature(context.Background(), projectKey, d.Key); err != nil {
			return nil, err
		}
	}
	f := fromSDKFeature(updated)
	res := &FeatureWriteResult{Feature: &f, Changed: true}
	if c.RemoveVariant != "" {
		res.Warnings = h.stagesServing(projectKey, d.Key, c.RemoveVariant)
	}
	return res, nil
}

// addVariantValues gives every flag its value for a new variant from
// <flag-key>=<value> pairs, parsed by each flag's type. Every flag needs one.
func addVariantValues(flags []FeatureFlag, variant string, pairs []string) error {
	given := map[string]string{}
	for _, pair := range pairs {
		k, raw, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return fmt.Errorf("invalid --value %q: want <flag-key>=<value>", pair)
		}
		if flagIndex(flags, k) < 0 {
			return fmt.Errorf("invalid --value %q: the feature has no flag %q", pair, k)
		}
		given[k] = raw
	}
	var missing []string
	for i := range flags {
		raw, ok := given[flags[i].Key]
		if !ok {
			missing = append(missing, flags[i].Key)
			continue
		}
		v, err := ParseFlagValue(flags[i].Type, raw)
		if err != nil {
			return fmt.Errorf("--value for flag %q: %w", flags[i].Key, err)
		}
		vals := map[string]any{}
		for k, existing := range flags[i].Variants {
			vals[k] = existing
		}
		vals[variant] = v
		flags[i].Variants = vals
	}
	if len(missing) > 0 {
		return fmt.Errorf("every flag needs a value for variant %q; pass --value <flag-key>=<value> for: %s", variant, strings.Join(missing, ", "))
	}
	return nil
}

// stagesServing names the stages whose targeting still serves a variant
// after it was removed. Their next targeting write fails until the rule
// changes. The read is best effort: without the targeting scope there is no
// warning.
func (h *Handler) stagesServing(projectKey, featureKey, variant string) []string {
	list, err := h.sdk.ListFeatureTargeting(context.Background(), projectKey, featureKey, sdkfm.ListOptions{})
	if err != nil {
		return nil
	}
	var out []string
	for _, ft := range list.Items {
		for i, r := range ft.Targeting.Rules {
			for _, s := range r.Serve {
				if s.VariantKey == variant {
					out = append(out, fmt.Sprintf("rule %d in stage %s still serves removed variant %s; change it before the stage's next targeting write", i, ft.ReleaseStageKey, variant))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// DeleteFeature deletes a feature, its flags and its targeting, and returns
// the key it deleted.
func (h *Handler) DeleteFeature(projectKey, keyOrName string) (string, error) {
	d, err := h.readFeature(projectKey, keyOrName)
	if err != nil {
		return "", err
	}
	return d.Key, h.sdk.DeleteFeature(context.Background(), projectKey, d.Key)
}

func flagIndex(flags []FeatureFlag, key string) int {
	for i, f := range flags {
		if f.Key == key {
			return i
		}
	}
	return -1
}

func variantIndex(vs []Variant, key string) int {
	for i, v := range vs {
		if v.Key == key {
			return i
		}
	}
	return -1
}

// sameJSON reports whether two request bodies encode the same.
func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	var va, vb any
	_ = json.Unmarshal(ja, &va)
	_ = json.Unmarshal(jb, &vb)
	return reflect.DeepEqual(va, vb)
}
