package featuremanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// ChangeRequestSettings controls whether a project's guarded release stages
// take targeting changes through change requests.
type ChangeRequestSettings struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// ProjectSettings are a project's settings.
type ProjectSettings struct {
	ChangeRequests *ChangeRequestSettings `json:"changeRequests,omitempty" yaml:"changeRequests,omitempty"`
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
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	var result ProjectList
	fetched := 0
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("list projects: stopped after %d pages", maxPages)
		}
		req := h.client.HTTP().R().SetContext(ctx).
			SetQueryParam("page", strconv.Itoa(page)).
			SetQueryParam("page-size", strconv.Itoa(pageSize))
		if opts.Sort != "" {
			req.SetQueryParam("sort", opts.Sort)
		}
		for _, f := range opts.AddFields {
			req.QueryParam.Add("add-fields", f)
		}

		resp, err := req.Get(BasePath + "/projects")
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		if err := httpclient.CheckResponse(resp); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		var pageResult ProjectList
		if err := json.Unmarshal(resp.Body(), &pageResult); err != nil {
			return nil, fmt.Errorf("list projects: parse response: %w", err)
		}

		if page == 1 {
			result.ResourceContext = pageResult.ResourceContext
		}
		result.Items = append(result.Items, pageResult.Items...)
		fetched += len(pageResult.Items)

		if lastPage(pageResult.Pagination, len(pageResult.Items), fetched, pageSize) {
			break
		}
	}
	if result.Items == nil {
		result.Items = []ProjectDetails{}
	}
	return &result, nil
}

// lastPage reports whether a page of n items ends a listing that has fetched
// items in total. The total count decides when the server sends one; a short
// or empty page decides otherwise. The page size the server reports wins over
// the requested one, since a server may cap it.
func lastPage(p *Pagination, n, fetched, requested int) bool {
	if n == 0 {
		return true
	}
	if p != nil && p.TotalCount != nil {
		return int64(fetched) >= *p.TotalCount
	}
	size := requested
	if p != nil && p.PageSize != nil && *p.PageSize > 0 {
		size = *p.PageSize
	}
	return n < size
}

// GetProject returns one project by key (getProject).
func (h *Handler) GetProject(ctx context.Context, projectKey string, addFields ...string) (*ProjectDetails, error) {
	req := h.client.HTTP().R().SetContext(ctx)
	for _, f := range addFields {
		req.QueryParam.Add("add-fields", f)
	}
	resp, err := req.Get(fmt.Sprintf("%s/projects/%s", BasePath, httpclient.PathSegment(projectKey)))
	if err != nil {
		return nil, fmt.Errorf("get project %q: %w", projectKey, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get project %q: %w", projectKey, err)
	}
	var result ProjectDetails
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get project %q: parse response: %w", projectKey, err)
	}
	return &result, nil
}
