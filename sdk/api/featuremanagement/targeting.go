package featuremanagement

import (
	"context"
	"fmt"
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// ConditionValueType is the value type of a targeting condition, which picks
// the operators it may use. Clients must tolerate values beyond the ones
// listed.
type ConditionValueType string

const (
	ConditionValueTypeString   ConditionValueType = "STRING"
	ConditionValueTypeNumber   ConditionValueType = "NUMBER"
	ConditionValueTypeBoolean  ConditionValueType = "BOOLEAN"
	ConditionValueTypeDateTime ConditionValueType = "DATE_TIME"
	ConditionValueTypeSemver   ConditionValueType = "SEM_VER"
)

// RolloutTransition is how a rollout reaches a step: at once (STEP, the
// default) or rising linearly from the previous step (LINEAR). Clients must
// tolerate values beyond the ones listed.
type RolloutTransition string

const (
	RolloutTransitionStep   RolloutTransition = "STEP"
	RolloutTransitionLinear RolloutTransition = "LINEAR"
)

// TargetingCondition is one condition of a targeting rule. The API has one
// schema per value type, discriminated by valueType; they share every field
// and differ only in the operators and value types they allow, so one struct
// carries them all. Values keeps each value as decoded: a string, number or
// boolean, and for DATE_TIME a string of epoch seconds.
type TargetingCondition struct {
	ValueType  ConditionValueType `json:"valueType" yaml:"valueType"`
	ContextKey string             `json:"contextKey" yaml:"contextKey"`
	Operator   string             `json:"operator" yaml:"operator"`
	Values     []any              `json:"values,omitempty" yaml:"values,omitempty"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// VariantDistribution is the share of a rule's matches served one variant.
type VariantDistribution struct {
	VariantKey string  `json:"variantKey" yaml:"variantKey"`
	Percentage float64 `json:"percentage" yaml:"percentage"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// RolloutStep is one step of a rollout: the percentage of a rule's matches
// that get the rule's variant from EffectiveTime on.
type RolloutStep struct {
	EffectiveTime time.Time         `json:"effectiveTime" yaml:"effectiveTime"`
	Percentage    float64           `json:"percentage" yaml:"percentage"`
	Transition    RolloutTransition `json:"transition,omitempty" yaml:"transition,omitempty"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// Rollout is a rule's percentage rollout schedule.
type Rollout struct {
	Steps []RolloutStep `json:"steps" yaml:"steps"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// TargetingRule is one rule of a stage's targeting. Rules are evaluated in
// order and the first match wins; a rule with no conditions matches every
// request. ID is service-generated and seeds bucketing, so it must survive
// every write unchanged.
type TargetingRule struct {
	ID           string                `json:"id,omitempty" yaml:"id,omitempty"`
	Name         string                `json:"name,omitempty" yaml:"name,omitempty"`
	Serve        []VariantDistribution `json:"serve,omitempty" yaml:"serve,omitempty"`
	Conditions   []TargetingCondition  `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Rollout      *Rollout              `json:"rollout,omitempty" yaml:"rollout,omitempty"`
	BucketingKey string                `json:"bucketingKey,omitempty" yaml:"bucketingKey,omitempty"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// Targeting is the ordered rule list of one stage.
type Targeting struct {
	Rules []TargetingRule `json:"rules" yaml:"rules"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// FeatureTargetingDetails is a feature's targeting in one release stage
// (schema FeatureReleaseStageTargetingDetails). Every stage of a feature has
// one; Enabled is the stage's on/off switch.
type FeatureTargetingDetails struct {
	ProjectKey       string           `json:"projectKey" yaml:"projectKey"`
	FeatureKey       string           `json:"featureKey" yaml:"featureKey"`
	ReleaseStageKey  string           `json:"releaseStageKey" yaml:"releaseStageKey"`
	Enabled          bool             `json:"enabled" yaml:"enabled"`
	Targeting        Targeting        `json:"targeting" yaml:"targeting"`
	Version          int              `json:"version" yaml:"version"`
	ModificationInfo ModificationInfo `json:"modificationInfo" yaml:"modificationInfo"`
	ResourceContext  *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// FeatureTargetingList is one page of a feature's per-stage targeting
// (schema FeatureReleaseStageTargetingList). ListFeatureTargeting returns
// every page merged into one, with Pagination unset.
type FeatureTargetingList struct {
	Items           []FeatureTargetingDetails `json:"items" yaml:"items"`
	Pagination      *Pagination               `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	ResourceContext *ResourceContext          `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ListFeatureTargeting returns a feature's targeting in every release stage
// (listFeatureReleaseStageTargeting).
func (h *Handler) ListFeatureTargeting(ctx context.Context, projectKey, featureKey string, opts ListOptions) (*FeatureTargetingList, error) {
	items, collection, err := listAll[FeatureTargetingDetails](ctx, h.client,
		fmt.Sprintf("list targeting of feature %q in project %q", featureKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey), "release-stage-targeting"), opts)
	if err != nil {
		return nil, err
	}
	return &FeatureTargetingList{Items: items, ResourceContext: collection}, nil
}

// GetFeatureTargeting returns a feature's targeting in one release stage
// (getFeatureReleaseStageTargeting).
func (h *Handler) GetFeatureTargeting(ctx context.Context, projectKey, featureKey, releaseStageKey string, addFields ...string) (*FeatureTargetingDetails, error) {
	return getOne[FeatureTargetingDetails](ctx, h.client,
		fmt.Sprintf("get targeting of feature %q in release stage %q of project %q", featureKey, releaseStageKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey), "release-stage-targeting", httpclient.PathSegment(releaseStageKey)), addFields)
}

// The targeting types are read and sent back on every targeting write, so
// each keeps the members it does not declare (see unknownFields).

func (c *TargetingCondition) UnmarshalJSON(data []byte) error {
	type plain TargetingCondition
	u, err := decodeKeepingUnknown(data, (*plain)(c))
	c.unknown = u
	return err
}

func (c TargetingCondition) MarshalJSON() ([]byte, error) {
	type plain TargetingCondition
	return encodeKeepingUnknown(plain(c), c.unknown)
}

func (d *VariantDistribution) UnmarshalJSON(data []byte) error {
	type plain VariantDistribution
	u, err := decodeKeepingUnknown(data, (*plain)(d))
	d.unknown = u
	return err
}

func (d VariantDistribution) MarshalJSON() ([]byte, error) {
	type plain VariantDistribution
	return encodeKeepingUnknown(plain(d), d.unknown)
}

func (s *RolloutStep) UnmarshalJSON(data []byte) error {
	type plain RolloutStep
	u, err := decodeKeepingUnknown(data, (*plain)(s))
	s.unknown = u
	return err
}

func (s RolloutStep) MarshalJSON() ([]byte, error) {
	type plain RolloutStep
	return encodeKeepingUnknown(plain(s), s.unknown)
}

func (r *Rollout) UnmarshalJSON(data []byte) error {
	type plain Rollout
	u, err := decodeKeepingUnknown(data, (*plain)(r))
	r.unknown = u
	return err
}

func (r Rollout) MarshalJSON() ([]byte, error) {
	type plain Rollout
	return encodeKeepingUnknown(plain(r), r.unknown)
}

func (r *TargetingRule) UnmarshalJSON(data []byte) error {
	type plain TargetingRule
	u, err := decodeKeepingUnknown(data, (*plain)(r))
	r.unknown = u
	return err
}

func (r TargetingRule) MarshalJSON() ([]byte, error) {
	type plain TargetingRule
	return encodeKeepingUnknown(plain(r), r.unknown)
}

func (t *Targeting) UnmarshalJSON(data []byte) error {
	type plain Targeting
	u, err := decodeKeepingUnknown(data, (*plain)(t))
	t.unknown = u
	return err
}

func (t Targeting) MarshalJSON() ([]byte, error) {
	type plain Targeting
	if t.Rules == nil {
		// rules is required: a stage with no rules is an empty list, never null.
		t.Rules = []TargetingRule{}
	}
	return encodeKeepingUnknown(plain(t), t.unknown)
}

// FeatureTargetingUpdate is the body of a targeting write (schema
// FeatureReleaseStageTargetingUpdate). It replaces the stage's whole
// targeting: send every rule back, existing ids unchanged, since a rule that
// loses its id gets a new one and re-buckets its users.
type FeatureTargetingUpdate struct {
	Enabled   bool      `json:"enabled" yaml:"enabled"`
	Targeting Targeting `json:"targeting" yaml:"targeting"`
}

// UpdateFeatureTargeting replaces a feature's targeting in one release stage
// (updateFeatureReleaseStageTargeting). optimisticLockingVersion is the
// version the caller read; the server answers 409 (httpclient.ErrConflict)
// when the stage changed since. An empty version sends none.
func (h *Handler) UpdateFeatureTargeting(ctx context.Context, projectKey, featureKey, releaseStageKey string, body FeatureTargetingUpdate, optimisticLockingVersion string) (*FeatureTargetingDetails, error) {
	return putOne[FeatureTargetingDetails](ctx, h.client,
		fmt.Sprintf("update targeting of feature %q in release stage %q of project %q", featureKey, releaseStageKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey), "release-stage-targeting", httpclient.PathSegment(releaseStageKey)),
		body, optimisticLockingVersion)
}
