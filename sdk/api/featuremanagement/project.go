package featuremanagement

import (
	"context"
	"fmt"
)

// ChangeRequestSettings controls whether a project's guarded release stages
// take targeting changes through change requests.
type ChangeRequestSettings struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// ProjectSettings are a project's settings.
type ProjectSettings struct {
	ChangeRequests *ChangeRequestSettings `json:"changeRequests,omitempty" yaml:"changeRequests,omitempty"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// ProjectDetails is the read model of a project (schema ProjectDetails).
type ProjectDetails struct {
	Key              string           `json:"key" yaml:"key"`
	Name             string           `json:"name" yaml:"name"`
	Description      string           `json:"description" yaml:"description"`
	Owner            string           `json:"owner" yaml:"owner"`
	Maintainers      []string         `json:"maintainers" yaml:"maintainers"`
	Settings         ProjectSettings  `json:"settings" yaml:"settings"`
	ModificationInfo ModificationInfo `json:"modificationInfo" yaml:"modificationInfo"`
	ResourceContext  *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
	// Version is the optimistic-locking version a write of this project
	// must send.
	Version int `json:"version" yaml:"version"`

	// unknown keeps members this type does not declare (decision 3).
	unknown unknownFields
}

// ProjectList is one page of projects (schema ProjectList). ListProjects
// returns every page merged into one, with Pagination unset.
type ProjectList struct {
	Items           []ProjectDetails `json:"items" yaml:"items"`
	Pagination      *Pagination      `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	ResourceContext *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// ListProjects returns every project in the environment (listProjects),
// following the API's page-number pagination to the last page.
func (h *Handler) ListProjects(ctx context.Context, opts ListOptions) (*ProjectList, error) {
	items, collection, err := listAll[ProjectDetails](ctx, h.client, "list projects", BasePath+"/projects", opts)
	if err != nil {
		return nil, err
	}
	return &ProjectList{Items: items, ResourceContext: collection}, nil
}

// GetProject returns one project by key (getProject).
func (h *Handler) GetProject(ctx context.Context, projectKey string, addFields ...string) (*ProjectDetails, error) {
	return getOne[ProjectDetails](ctx, h.client, fmt.Sprintf("get project %q", projectKey), projectPath(projectKey), addFields)
}
