package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// ReleaseStageType is the kind of environment a release stage stands for.
// Clients must tolerate values beyond the ones listed.
type ReleaseStageType string

const (
	ReleaseStageTypeDevelopment ReleaseStageType = "DEVELOPMENT"
	ReleaseStageTypeStaging     ReleaseStageType = "STAGING"
	ReleaseStageTypeProduction  ReleaseStageType = "PRODUCTION"
)

// ReleaseStageDetails is the read model of a release stage (schema
// ReleaseStageDetails).
type ReleaseStageDetails struct {
	Key         string           `json:"key" yaml:"key"`
	Name        string           `json:"name" yaml:"name"`
	Description string           `json:"description" yaml:"description"`
	Type        ReleaseStageType `json:"type" yaml:"type"`
	// Guarded release stages take targeting changes through change requests
	// when the project has change requests enabled.
	Guarded *bool `json:"guarded,omitempty" yaml:"guarded,omitempty"`
	// DisplayOrder is the stage's position in the project, lowest first.
	DisplayOrder     int              `json:"displayOrder" yaml:"displayOrder"`
	ModificationInfo ModificationInfo `json:"modificationInfo" yaml:"modificationInfo"`
	ResourceContext  *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
	Version          int              `json:"version" yaml:"version"`
}

// ReleaseStageList is one page of release stages (schema ReleaseStageList).
// ListReleaseStages returns every page merged into one, with Pagination unset.
type ReleaseStageList struct {
	Items           []ReleaseStageDetails `json:"items" yaml:"items"`
	Pagination      *Pagination           `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	ResourceContext *ResourceContext      `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ListReleaseStages returns every release stage in a project
// (listReleaseStages).
func (h *Handler) ListReleaseStages(ctx context.Context, projectKey string, opts ListOptions) (*ReleaseStageList, error) {
	items, collection, err := listAll[ReleaseStageDetails](ctx, h.client,
		fmt.Sprintf("list release stages of project %q", projectKey), projectPath(projectKey, "release-stages"), opts)
	if err != nil {
		return nil, err
	}
	return &ReleaseStageList{Items: items, ResourceContext: collection}, nil
}

// GetReleaseStage returns one release stage by key (getReleaseStage).
func (h *Handler) GetReleaseStage(ctx context.Context, projectKey, releaseStageKey string, addFields ...string) (*ReleaseStageDetails, error) {
	return getOne[ReleaseStageDetails](ctx, h.client,
		fmt.Sprintf("get release stage %q of project %q", releaseStageKey, projectKey),
		projectPath(projectKey, "release-stages", httpclient.PathSegment(releaseStageKey)), addFields)
}
