package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// FeatureType is what a feature is for. Clients must tolerate values beyond
// the ones listed.
type FeatureType string

const (
	FeatureTypeRelease       FeatureType = "RELEASE"
	FeatureTypeConfiguration FeatureType = "CONFIGURATION"
	FeatureTypeEntitlements  FeatureType = "ENTITLEMENTS"
	FeatureTypeExperiment    FeatureType = "EXPERIMENT"
	FeatureTypeOps           FeatureType = "OPS"
)

// FeatureStatus is a feature's lifecycle status.
type FeatureStatus string

const (
	FeatureStatusActive   FeatureStatus = "ACTIVE"
	FeatureStatusArchived FeatureStatus = "ARCHIVED"
)

// FlagType is the type of a flag's values.
type FlagType string

const (
	FlagTypeBoolean FlagType = "BOOLEAN"
	FlagTypeJSON    FlagType = "JSON"
	FlagTypeNumber  FlagType = "NUMBER"
	FlagTypeString  FlagType = "STRING"
)

// Variant is one variant of a feature (schema VariantDetails).
type Variant struct {
	Key             string           `json:"key" yaml:"key"`
	Name            string           `json:"name" yaml:"name"`
	DisplayOrder    *int             `json:"displayOrder,omitempty" yaml:"displayOrder,omitempty"`
	ResourceContext *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// FeatureFlag is one flag of a feature (schema FeatureFlag): a typed value
// with one entry per variant, which is what SDKs evaluate.
type FeatureFlag struct {
	Key         string   `json:"key" yaml:"key"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Type        FlagType `json:"type" yaml:"type"`
	// Variants maps a variant key to the flag's value for it: a boolean,
	// number, string or JSON object, matching Type.
	Variants map[string]any `json:"variants" yaml:"variants"`
	// Schema constrains the flag's values. It is one of several shapes,
	// discriminated by its "type" field, and is kept as decoded.
	Schema map[string]any `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// FeatureDetails is the read model of a feature (schema FeatureDetails).
type FeatureDetails struct {
	ProjectKey       string           `json:"projectKey" yaml:"projectKey"`
	Key              string           `json:"key" yaml:"key"`
	Name             string           `json:"name" yaml:"name"`
	Description      string           `json:"description" yaml:"description"`
	Variants         []Variant        `json:"variants" yaml:"variants"`
	Flags            []FeatureFlag    `json:"flags" yaml:"flags"`
	Owner            string           `json:"owner" yaml:"owner"`
	Maintainers      []string         `json:"maintainers" yaml:"maintainers"`
	ModificationInfo ModificationInfo `json:"modificationInfo" yaml:"modificationInfo"`
	Status           FeatureStatus    `json:"status" yaml:"status"`
	Type             FeatureType      `json:"type" yaml:"type"`
	ResourceContext  *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
	Version          int              `json:"version" yaml:"version"`
}

// FeatureList is one page of features (schema FeatureList). ListFeatures
// returns every page merged into one, with Pagination unset.
type FeatureList struct {
	Items           []FeatureDetails `json:"items" yaml:"items"`
	Pagination      *Pagination      `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	ResourceContext *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ListFeatures returns every feature in a project (listFeatures).
func (h *Handler) ListFeatures(ctx context.Context, projectKey string, opts ListOptions) (*FeatureList, error) {
	items, collection, err := listAll[FeatureDetails](ctx, h.client,
		fmt.Sprintf("list features of project %q", projectKey), projectPath(projectKey, "features"), opts)
	if err != nil {
		return nil, err
	}
	return &FeatureList{Items: items, ResourceContext: collection}, nil
}

// GetFeature returns one feature by key, including its variants and flags
// (getFeature).
func (h *Handler) GetFeature(ctx context.Context, projectKey, featureKey string, addFields ...string) (*FeatureDetails, error) {
	return getOne[FeatureDetails](ctx, h.client,
		fmt.Sprintf("get feature %q of project %q", featureKey, projectKey),
		projectPath(projectKey, "features", httpclient.PathSegment(featureKey)), addFields)
}
