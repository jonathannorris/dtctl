package featuremanagement

import (
	"context"
	"sort"
	"strconv"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// Re-exported SDK feature types.
type (
	FeatureType   = sdkfm.FeatureType
	FeatureStatus = sdkfm.FeatureStatus
	FlagType      = sdkfm.FlagType
	Variant       = sdkfm.Variant
	FeatureFlag   = sdkfm.FeatureFlag
)

// Feature is the CLI view of a feature. JSON and YAML keys are the API's own,
// in the API's order; the *Display fields exist only for the table.
type Feature struct {
	ProjectKey          string           `json:"projectKey" yaml:"projectKey" table:"-"`
	Key                 string           `json:"key" yaml:"key" table:"KEY"`
	Name                string           `json:"name" yaml:"name" table:"NAME"`
	Description         string           `json:"description" yaml:"description" table:"DESCRIPTION,wide"`
	Variants            []Variant        `json:"variants" yaml:"variants" table:"-"`
	VariantsDisplay     string           `json:"-" yaml:"-" table:"VARIANTS,literal"`
	Flags               []FeatureFlag    `json:"flags" yaml:"flags" table:"-"`
	FlagsDisplay        string           `json:"-" yaml:"-" table:"FLAGS"`
	Owner               string           `json:"owner" yaml:"owner" table:"OWNER,wide"`
	Maintainers         []string         `json:"maintainers" yaml:"maintainers" table:"-"`
	ModificationInfo    ModificationInfo `json:"modificationInfo" yaml:"modificationInfo" table:"-"`
	Status              FeatureStatus    `json:"status" yaml:"status" table:"STATUS"`
	Type                FeatureType      `json:"type" yaml:"type" table:"TYPE"`
	ResourceContext     *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty" table:"-"`
	LastModifiedDisplay string           `json:"-" yaml:"-" table:"LAST_MODIFIED,wide"`
	Version             int              `json:"version" yaml:"version" table:"VERSION,wide"`
}

func fromSDKFeature(s *sdkfm.FeatureDetails) Feature {
	f := Feature{
		ProjectKey:       s.ProjectKey,
		Key:              s.Key,
		Name:             s.Name,
		Description:      s.Description,
		Variants:         s.Variants,
		Flags:            s.Flags,
		Owner:            s.Owner,
		Maintainers:      s.Maintainers,
		ModificationInfo: s.ModificationInfo,
		Status:           s.Status,
		Type:             s.Type,
		ResourceContext:  s.ResourceContext,
		Version:          s.Version,
	}
	if f.Variants == nil {
		f.Variants = []Variant{}
	}
	if f.Flags == nil {
		f.Flags = []FeatureFlag{}
	}
	if f.Maintainers == nil {
		f.Maintainers = []string{}
	}
	sortVariants(f.Variants)
	keys := make([]string, len(f.Variants))
	for i, v := range f.Variants {
		keys[i] = v.Key
	}
	f.VariantsDisplay = strings.Join(keys, ",")
	f.FlagsDisplay = strconv.Itoa(len(f.Flags))
	f.LastModifiedDisplay = displayTime(s.ModificationInfo.LastModifiedTime)
	return f
}

// sortVariants puts variants in their display order, the order the UI shows
// them in; variants without one keep their place after the ordered ones.
func sortVariants(vs []Variant) {
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := vs[i].DisplayOrder, vs[j].DisplayOrder
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a < *b
	})
}

// ListFeatures returns every feature in a project, sorted by key.
func (h *Handler) ListFeatures(projectKey string) ([]Feature, error) {
	list, err := h.sdk.ListFeatures(context.Background(), projectKey, sdkfm.ListOptions{Sort: "key"})
	if err != nil {
		return nil, err
	}
	features := make([]Feature, len(list.Items))
	for i := range list.Items {
		features[i] = fromSDKFeature(&list.Items[i])
	}
	sort.SliceStable(features, func(i, j int) bool { return features[i].Key < features[j].Key })
	return features, nil
}

// GetFeature returns one feature of a project by key or name, with its
// variants and flags.
func (h *Handler) GetFeature(projectKey, keyOrName string) (*Feature, error) {
	f, err := resolveKeyOrName("feature", keyOrName,
		func(key string) (Feature, error) {
			s, err := h.sdk.GetFeature(context.Background(), projectKey, key)
			if err != nil {
				return Feature{}, err
			}
			return fromSDKFeature(s), nil
		},
		func() ([]Feature, error) { return h.ListFeatures(projectKey) },
		func(f Feature) string { return f.Key },
		func(f Feature) string { return f.Name },
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}
