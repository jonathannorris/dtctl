package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// ContextAttributeType is the value type of a context attribute, which is the
// value type of every targeting condition on its context key. Clients must
// tolerate values beyond the ones listed.
type ContextAttributeType string

const (
	ContextAttributeTypeBoolean  ContextAttributeType = "BOOLEAN"
	ContextAttributeTypeDateTime ContextAttributeType = "DATE_TIME"
	ContextAttributeTypeNumber   ContextAttributeType = "NUMBER"
	ContextAttributeTypeSemver   ContextAttributeType = "SEM_VER"
	ContextAttributeTypeString   ContextAttributeType = "STRING"
)

// AddFieldUsagesCount asks a context attribute read to include UsageCount.
// (The parameter value is "usagesCount"; the field it adds is "usageCount".)
const AddFieldUsagesCount = "usagesCount"

// AddFieldResourceContext asks a read to include the caller's permitted
// operations.
const AddFieldResourceContext = "resourceContext"

// ContextAttributeDetails is the read model of a context attribute (schema
// ContextAttributeDetails): a declared key of the evaluation context, such as
// plan.tier, that targeting rules can match on.
type ContextAttributeDetails struct {
	Key         string               `json:"key" yaml:"key"`
	Name        string               `json:"name" yaml:"name"`
	Description string               `json:"description" yaml:"description"`
	Schema      string               `json:"schema" yaml:"schema"`
	Type        ContextAttributeType `json:"type" yaml:"type"`
	// ContextKey is the key in the evaluation context that SDKs send and
	// targeting conditions name.
	ContextKey       string           `json:"contextKey" yaml:"contextKey"`
	ProjectKey       string           `json:"projectKey" yaml:"projectKey"`
	ModificationInfo ModificationInfo `json:"modificationInfo" yaml:"modificationInfo"`
	ResourceContext  *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
	Version          int              `json:"version" yaml:"version"`
	// UsageCount is the number of targeting rules that reference the
	// attribute, present only when AddFieldUsagesCount was requested.
	UsageCount *int `json:"usageCount,omitempty" yaml:"usageCount,omitempty"`
	// BuiltIn attributes can't be modified or deleted.
	BuiltIn bool `json:"builtIn" yaml:"builtIn"`
}

// ContextAttributeList is one page of context attributes (schema
// ContextAttributeList). ListContextAttributes returns every page merged into
// one, with Pagination unset.
type ContextAttributeList struct {
	Items           []ContextAttributeDetails `json:"items" yaml:"items"`
	Pagination      *Pagination               `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	ResourceContext *ResourceContext          `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ContextAttributeUsageReleaseStage is one release stage whose targeting
// references a context attribute.
type ContextAttributeUsageReleaseStage struct {
	ReleaseStageKey  string `json:"releaseStageKey" yaml:"releaseStageKey"`
	ReleaseStageName string `json:"releaseStageName" yaml:"releaseStageName"`
}

// ContextAttributeUsage is one feature whose targeting references a context
// attribute, with the stages it does so in (schema ContextAttributeUsage).
type ContextAttributeUsage struct {
	FeatureKey    string                              `json:"featureKey" yaml:"featureKey"`
	FeatureName   string                              `json:"featureName" yaml:"featureName"`
	ReleaseStages []ContextAttributeUsageReleaseStage `json:"releaseStages" yaml:"releaseStages"`
}

// ContextAttributeUsageList is the usage of one context attribute (schema
// ContextAttributeUsageList). It is not paged.
type ContextAttributeUsageList struct {
	Items           []ContextAttributeUsage `json:"items" yaml:"items"`
	ResourceContext *ResourceContext        `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ListContextAttributes returns every context attribute in a project
// (listContextAttributes).
func (h *Handler) ListContextAttributes(ctx context.Context, projectKey string, opts ListOptions) (*ContextAttributeList, error) {
	items, collection, err := listAll[ContextAttributeDetails](ctx, h.client,
		fmt.Sprintf("list context attributes of project %q", projectKey), projectPath(projectKey, "context-attributes"), opts)
	if err != nil {
		return nil, err
	}
	return &ContextAttributeList{Items: items, ResourceContext: collection}, nil
}

// GetContextAttribute returns one context attribute by key
// (getContextAttribute).
func (h *Handler) GetContextAttribute(ctx context.Context, projectKey, key string, addFields ...string) (*ContextAttributeDetails, error) {
	return getOne[ContextAttributeDetails](ctx, h.client,
		fmt.Sprintf("get context attribute %q of project %q", key, projectKey),
		projectPath(projectKey, "context-attributes", httpclient.PathSegment(key)), addFields)
}

// GetContextAttributeUsage returns the features, and the stages within them,
// whose targeting references a context attribute (getContextAttributeUsage).
func (h *Handler) GetContextAttributeUsage(ctx context.Context, projectKey, key string) (*ContextAttributeUsageList, error) {
	return getOne[ContextAttributeUsageList](ctx, h.client,
		fmt.Sprintf("get usage of context attribute %q of project %q", key, projectKey),
		projectPath(projectKey, "context-attributes", httpclient.PathSegment(key), "usage"), nil)
}
