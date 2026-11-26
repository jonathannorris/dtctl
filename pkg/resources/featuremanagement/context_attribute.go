package featuremanagement

import (
	"context"
	"sort"
	"strconv"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// Re-exported SDK context attribute types.
type (
	ContextAttributeType              = sdkfm.ContextAttributeType
	ContextAttributeUsage             = sdkfm.ContextAttributeUsage
	ContextAttributeUsageReleaseStage = sdkfm.ContextAttributeUsageReleaseStage
)

// ContextAttribute is the CLI view of a context attribute. JSON and YAML keys
// are the API's own, in the API's order; the *Display fields exist only for
// the table.
type ContextAttribute struct {
	Key                 string               `json:"key" yaml:"key" table:"KEY"`
	Name                string               `json:"name" yaml:"name" table:"NAME"`
	Description         string               `json:"description" yaml:"description" table:"DESCRIPTION,wide"`
	Schema              string               `json:"schema" yaml:"schema" table:"-"`
	Type                ContextAttributeType `json:"type" yaml:"type" table:"TYPE"`
	ContextKey          string               `json:"contextKey" yaml:"contextKey" table:"CONTEXT_KEY"`
	ProjectKey          string               `json:"projectKey" yaml:"projectKey" table:"-"`
	ModificationInfo    ModificationInfo     `json:"modificationInfo" yaml:"modificationInfo" table:"-"`
	ResourceContext     *ResourceContext     `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty" table:"-"`
	Version             int                  `json:"version" yaml:"version" table:"VERSION,wide"`
	UsageCount          *int                 `json:"usageCount,omitempty" yaml:"usageCount,omitempty" table:"-"`
	UsagesDisplay       string               `json:"-" yaml:"-" table:"RULES"`
	BuiltIn             bool                 `json:"builtIn" yaml:"builtIn" table:"-"`
	BuiltInDisplay      string               `json:"-" yaml:"-" table:"BUILT_IN"`
	LastModifiedDisplay string               `json:"-" yaml:"-" table:"LAST_MODIFIED,wide"`
}

func fromSDKContextAttribute(s *sdkfm.ContextAttributeDetails) ContextAttribute {
	a := ContextAttribute{
		Key:              s.Key,
		Name:             s.Name,
		Description:      s.Description,
		Schema:           s.Schema,
		Type:             s.Type,
		ContextKey:       s.ContextKey,
		ProjectKey:       s.ProjectKey,
		ModificationInfo: s.ModificationInfo,
		ResourceContext:  s.ResourceContext,
		Version:          s.Version,
		UsageCount:       s.UsageCount,
		BuiltIn:          s.BuiltIn,
	}
	a.UsagesDisplay = "-"
	if a.UsageCount != nil {
		a.UsagesDisplay = strconv.Itoa(*a.UsageCount)
	}
	a.BuiltInDisplay = yesNo(a.BuiltIn)
	a.LastModifiedDisplay = displayTime(s.ModificationInfo.LastModifiedTime)
	return a
}

// ListContextAttributes returns every context attribute in a project, with
// the number of targeting rules that reference each, sorted by key.
func (h *Handler) ListContextAttributes(projectKey string) ([]ContextAttribute, error) {
	list, err := h.sdk.ListContextAttributes(context.Background(), projectKey, sdkfm.ListOptions{
		Sort: "key", AddFields: []string{sdkfm.AddFieldUsagesCount},
	})
	if err != nil {
		return nil, err
	}
	attrs := make([]ContextAttribute, len(list.Items))
	for i := range list.Items {
		attrs[i] = fromSDKContextAttribute(&list.Items[i])
	}
	sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
	return attrs, nil
}

// GetContextAttribute returns one context attribute of a project by key or
// name, with its rule count.
func (h *Handler) GetContextAttribute(projectKey, keyOrName string) (*ContextAttribute, error) {
	a, err := resolveKeyOrName("context attribute", keyOrName,
		func(key string) (ContextAttribute, error) {
			s, err := h.sdk.GetContextAttribute(context.Background(), projectKey, key, sdkfm.AddFieldUsagesCount)
			if err != nil {
				return ContextAttribute{}, err
			}
			return fromSDKContextAttribute(s), nil
		},
		func() ([]ContextAttribute, error) { return h.ListContextAttributes(projectKey) },
		func(a ContextAttribute) string { return a.Key },
		func(a ContextAttribute) string { return a.Name },
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetContextAttributeUsage returns the features whose targeting references a
// context attribute, by key, sorted by feature key.
func (h *Handler) GetContextAttributeUsage(projectKey, key string) ([]ContextAttributeUsage, error) {
	list, err := h.sdk.GetContextAttributeUsage(context.Background(), projectKey, key)
	if err != nil {
		return nil, err
	}
	items := list.Items
	sort.SliceStable(items, func(i, j int) bool { return items[i].FeatureKey < items[j].FeatureKey })
	return items, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
