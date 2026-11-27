package featuremanagement

import (
	"context"
	"sort"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// ReleaseStageType re-exports the SDK's release stage type.
type ReleaseStageType = sdkfm.ReleaseStageType

// ReleaseStage is the CLI view of a release stage. JSON and YAML keys are the
// API's own, in the API's order; the *Display fields exist only for the table.
type ReleaseStage struct {
	Key                 string           `json:"key" yaml:"key" table:"KEY"`
	Name                string           `json:"name" yaml:"name" table:"NAME"`
	Description         string           `json:"description" yaml:"description" table:"DESCRIPTION,wide"`
	Type                ReleaseStageType `json:"type" yaml:"type" table:"TYPE"`
	Guarded             *bool            `json:"guarded,omitempty" yaml:"guarded,omitempty" table:"-"`
	GuardedDisplay      string           `json:"-" yaml:"-" table:"GUARDED"`
	DisplayOrder        int              `json:"displayOrder" yaml:"displayOrder" table:"ORDER,wide"`
	ModificationInfo    ModificationInfo `json:"modificationInfo" yaml:"modificationInfo" table:"-"`
	LastModifiedDisplay string           `json:"-" yaml:"-" table:"LAST_MODIFIED,wide"`
	ResourceContext     *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty" table:"-"`
	Version             int              `json:"version" yaml:"version" table:"VERSION,wide"`
}

// IsGuarded reports the stage's guarded setting, which the API defaults to
// false when absent.
func (r *ReleaseStage) IsGuarded() bool { return r.Guarded != nil && *r.Guarded }

func fromSDKReleaseStage(s *sdkfm.ReleaseStageDetails) ReleaseStage {
	r := ReleaseStage{
		Key:              s.Key,
		Name:             s.Name,
		Description:      s.Description,
		Type:             s.Type,
		Guarded:          s.Guarded,
		DisplayOrder:     s.DisplayOrder,
		ModificationInfo: s.ModificationInfo,
		ResourceContext:  s.ResourceContext,
		Version:          s.Version,
	}
	r.GuardedDisplay = onOff(r.IsGuarded())
	r.LastModifiedDisplay = displayTime(s.ModificationInfo.LastModifiedTime)
	return r
}

// ListReleaseStages returns a project's release stages in their display
// order, which is the order a feature is released through them. The API
// cannot sort by it, so the order is applied here, ties broken by key.
func (h *Handler) ListReleaseStages(projectKey string) ([]ReleaseStage, error) {
	list, err := h.sdk.ListReleaseStages(context.Background(), projectKey, sdkfm.ListOptions{})
	if err != nil {
		return nil, err
	}
	stages := make([]ReleaseStage, len(list.Items))
	for i := range list.Items {
		stages[i] = fromSDKReleaseStage(&list.Items[i])
	}
	sort.SliceStable(stages, func(i, j int) bool {
		if stages[i].DisplayOrder != stages[j].DisplayOrder {
			return stages[i].DisplayOrder < stages[j].DisplayOrder
		}
		return stages[i].Key < stages[j].Key
	})
	return stages, nil
}

// GetReleaseStage returns one release stage of a project by key or name.
func (h *Handler) GetReleaseStage(projectKey, keyOrName string) (*ReleaseStage, error) {
	r, err := resolveKeyOrName("release stage", keyOrName,
		func(key string) (ReleaseStage, error) {
			s, err := h.sdk.GetReleaseStage(context.Background(), projectKey, key)
			if err != nil {
				return ReleaseStage{}, err
			}
			return fromSDKReleaseStage(s), nil
		},
		func() ([]ReleaseStage, error) { return h.ListReleaseStages(projectKey) },
		func(r ReleaseStage) string { return r.Key },
		func(r ReleaseStage) string { return r.Name },
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
