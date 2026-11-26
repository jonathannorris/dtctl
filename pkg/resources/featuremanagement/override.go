package featuremanagement

import (
	"context"
	"sort"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// Re-exported SDK override types. They print as the API returns them under
// -o json|yaml; the table views are built from the *Row types.
type (
	MyOverride             = sdkfm.MyOverride
	FeatureMyOverrides     = sdkfm.FeatureMyOverrides
	ReleaseStageMyOverride = sdkfm.ReleaseStageMyOverride
)

// FeatureOverrides is the caller's overrides of one feature, one per release
// stage. FeatureKey is added by dtctl, since the API's response does not
// name the feature.
type FeatureOverrides struct {
	FeatureKey string       `json:"featureKey" yaml:"featureKey"`
	Overrides  []MyOverride `json:"overrides" yaml:"overrides"`
}

// OverridesSummary is everything `describe fm overrides` shows: the caller's
// targeting keys (the overrides profile) and every override they have in the
// project, under the API's own field names.
type OverridesSummary struct {
	TargetingKeys []string             `json:"targetingKeys" yaml:"targetingKeys"`
	Items         []FeatureMyOverrides `json:"items" yaml:"items"`
}

// ProjectOverrideRow is one table row of `get fm overrides`: one feature in
// one stage.
type ProjectOverrideRow struct {
	Feature     string `table:"FEATURE"`
	FeatureName string `table:"FEATURE_NAME,wide"`
	Stage       string `table:"STAGE"`
	Variant     string `table:"VARIANT"`
	VariantName string `table:"VARIANT_NAME,wide"`
}

// ProjectOverrideRows flattens a project's overrides into table rows.
func ProjectOverrideRows(items []FeatureMyOverrides) []ProjectOverrideRow {
	rows := []ProjectOverrideRow{}
	for _, f := range items {
		for _, o := range f.Overrides {
			rows = append(rows, ProjectOverrideRow{
				Feature: f.FeatureKey, FeatureName: f.FeatureName,
				Stage: o.ReleaseStageKey, Variant: o.VariantKey, VariantName: o.VariantName,
			})
		}
	}
	return rows
}

// FeatureOverrideRow is one table row of `get fm override <feature>`: one
// release stage, with "-" where the caller has no override.
type FeatureOverrideRow struct {
	Stage        string `table:"STAGE"`
	Variant      string `table:"VARIANT"`
	LastModified string `table:"LAST_MODIFIED"`
}

// FeatureOverrideRows turns a feature's per-stage overrides into table rows.
func FeatureOverrideRows(o *FeatureOverrides) []FeatureOverrideRow {
	rows := make([]FeatureOverrideRow, len(o.Overrides))
	for i, ov := range o.Overrides {
		rows[i] = FeatureOverrideRow{Stage: ov.ReleaseStageKey, Variant: "-", LastModified: "-"}
		if ov.VariantKey != nil {
			rows[i].Variant = *ov.VariantKey
		}
		if ov.LastModifiedTime != nil {
			rows[i].LastModified = displayTime(*ov.LastModifiedTime)
		}
	}
	return rows
}

// ListOverrides returns every override the caller has in a project, by
// feature key.
func (h *Handler) ListOverrides(projectKey string) ([]FeatureMyOverrides, error) {
	all, err := h.sdk.GetProjectMyOverrides(context.Background(), projectKey)
	if err != nil {
		return nil, err
	}
	items := all.Items
	if items == nil {
		items = []FeatureMyOverrides{}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].FeatureKey < items[j].FeatureKey })
	return items, nil
}

// GetFeatureOverrides returns the caller's overrides of one feature, one per
// release stage. The feature is a key or, when no feature has that key, a
// name.
func (h *Handler) GetFeatureOverrides(projectKey, featureKeyOrName string) (*FeatureOverrides, error) {
	var featureKey string
	o, err := withFeatureKey(h, projectKey, featureKeyOrName, func(key string) (*sdkfm.MyOverrides, error) {
		featureKey = key
		return h.sdk.GetMyOverrides(context.Background(), projectKey, key)
	})
	if err != nil {
		return nil, err
	}
	overrides := o.Overrides
	if overrides == nil {
		overrides = []MyOverride{}
	}
	return &FeatureOverrides{FeatureKey: featureKey, Overrides: overrides}, nil
}

// GetOverridesSummary returns the caller's targeting keys and every override
// they have in a project.
func (h *Handler) GetOverridesSummary(projectKey string) (*OverridesSummary, error) {
	profile, err := h.sdk.GetOverridesProfile(context.Background(), projectKey)
	if err != nil {
		return nil, err
	}
	items, err := h.ListOverrides(projectKey)
	if err != nil {
		return nil, err
	}
	keys := profile.TargetingKeys
	if keys == nil {
		keys = []string{}
	}
	return &OverridesSummary{TargetingKeys: keys, Items: items}, nil
}
