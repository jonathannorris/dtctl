package featuremanagement

import (
	"context"
	"fmt"
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// The override operations act on the authenticated caller's own overrides:
// variant overrides that apply only to evaluations carrying one of the
// caller's targeting keys (the overrides profile).

// MyOverride is the caller's override of one feature in one release stage
// (schema MyOverride). VariantKey and LastModifiedTime are nil when the
// caller has no override in that stage.
type MyOverride struct {
	ReleaseStageKey  string     `json:"releaseStageKey" yaml:"releaseStageKey"`
	VariantKey       *string    `json:"variantKey" yaml:"variantKey"`
	LastModifiedTime *time.Time `json:"lastModifiedTime" yaml:"lastModifiedTime"`
}

// MyOverrides is the caller's overrides of one feature, one entry per
// release stage of the project (schema MyOverrides).
type MyOverrides struct {
	Overrides []MyOverride `json:"overrides" yaml:"overrides"`
}

// ReleaseStageMyOverride is one stage in which the caller overrides a
// feature (schema ReleaseStageMyOverride).
type ReleaseStageMyOverride struct {
	ReleaseStageKey  string `json:"releaseStageKey" yaml:"releaseStageKey"`
	ReleaseStageName string `json:"releaseStageName" yaml:"releaseStageName"`
	VariantKey       string `json:"variantKey" yaml:"variantKey"`
	VariantName      string `json:"variantName" yaml:"variantName"`
}

// FeatureMyOverrides is one feature the caller overrides, with the stages
// it does so in (schema FeatureMyOverrides).
type FeatureMyOverrides struct {
	FeatureKey  string                   `json:"featureKey" yaml:"featureKey"`
	FeatureName string                   `json:"featureName" yaml:"featureName"`
	Overrides   []ReleaseStageMyOverride `json:"overrides" yaml:"overrides"`
}

// ProjectMyOverrides is every override the caller has in a project, grouped
// by feature; features and stages without one are left out (schema
// ProjectMyOverrides).
type ProjectMyOverrides struct {
	Items []FeatureMyOverrides `json:"items" yaml:"items"`
}

// OverridesProfile is the caller's targeting keys in a project (schema
// OverridesProfile): the values their overrides apply to, case-sensitive, as
// passed in SDK evaluation calls.
type OverridesProfile struct {
	TargetingKeys []string `json:"targetingKeys" yaml:"targetingKeys"`
}

// GetProjectMyOverrides returns every override the caller has in a project
// (getAllFeatureOverrides).
func (h *Handler) GetProjectMyOverrides(ctx context.Context, projectKey string) (*ProjectMyOverrides, error) {
	return getOne[ProjectMyOverrides](ctx, h.client,
		fmt.Sprintf("get your overrides in project %q", projectKey), projectPath(projectKey, "my-overrides"), nil)
}

// GetMyOverrides returns the caller's overrides of one feature, one per
// release stage (getMyOverrides).
func (h *Handler) GetMyOverrides(ctx context.Context, projectKey, featureKey string) (*MyOverrides, error) {
	return getOne[MyOverrides](ctx, h.client,
		fmt.Sprintf("get your overrides of feature %q in project %q", featureKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey), "my-overrides"), nil)
}

// GetOverridesProfile returns the caller's targeting keys in a project
// (getOverridesProfile).
func (h *Handler) GetOverridesProfile(ctx context.Context, projectKey string) (*OverridesProfile, error) {
	return getOne[OverridesProfile](ctx, h.client,
		fmt.Sprintf("get your overrides profile in project %q", projectKey), projectPath(projectKey, "overrides-profile"), nil)
}
