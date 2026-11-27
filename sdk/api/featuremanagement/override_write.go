package featuremanagement

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// MyOverrideUpdate is one stage's override in a MyOverridesUpdate: the
// variant served to the caller there, or nil for none.
type MyOverrideUpdate struct {
	ReleaseStageKey string  `json:"releaseStageKey" yaml:"releaseStageKey"`
	VariantKey      *string `json:"variantKey" yaml:"variantKey"`
}

// MyOverridesUpdate is the full set of the caller's overrides of one
// feature (schema MyOverridesUpdate). A stage left out loses its override.
type MyOverridesUpdate struct {
	Overrides []MyOverrideUpdate `json:"overrides" yaml:"overrides"`
}

// MarshalJSON sends an empty list rather than null: the API rejects a null
// overrides member instead of reading it as "remove them all".
func (u MyOverridesUpdate) MarshalJSON() ([]byte, error) {
	type plain MyOverridesUpdate
	if u.Overrides == nil {
		u.Overrides = []MyOverrideUpdate{}
	}
	return json.Marshal(plain(u))
}

// UpdateMyOverrides replaces the caller's overrides of one feature
// (updateMyOverrides).
func (h *Handler) UpdateMyOverrides(ctx context.Context, projectKey, featureKey string, body MyOverridesUpdate) (*MyOverrides, error) {
	return putOne[MyOverrides](ctx, h.client,
		fmt.Sprintf("update your overrides of feature %q in project %q", featureKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey), "my-overrides"), body, "")
}

// DeleteProjectMyOverrides deletes every override the caller has in a
// project (deleteMyOverrides).
func (h *Handler) DeleteProjectMyOverrides(ctx context.Context, projectKey string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete your overrides in project %q", projectKey), projectPath(projectKey, "my-overrides"))
}

// UpdateOverridesProfile replaces the caller's targeting keys in a project
// (updateOverridesProfile). The API answers without the profile; read it
// back with GetOverridesProfile.
func (h *Handler) UpdateOverridesProfile(ctx context.Context, projectKey string, body OverridesProfile) error {
	if body.TargetingKeys == nil {
		body.TargetingKeys = []string{}
	}
	_, err := putOne[json.RawMessage](ctx, h.client,
		fmt.Sprintf("update your overrides profile in project %q", projectKey), projectPath(projectKey, "overrides-profile"), body, "")
	return err
}

// DeleteOverridesProfile deletes the caller's targeting keys in a project
// (deleteOverridesProfile).
func (h *Handler) DeleteOverridesProfile(ctx context.Context, projectKey string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete your overrides profile in project %q", projectKey), projectPath(projectKey, "overrides-profile"))
}
