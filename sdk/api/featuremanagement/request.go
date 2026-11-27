package featuremanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// page is one page of any Feature Management list response: every list schema
// (ProjectList, ReleaseStageList, ...) has this shape.
type page[T any] struct {
	Items           []T              `json:"items"`
	Pagination      *Pagination      `json:"pagination,omitempty"`
	ResourceContext *ResourceContext `json:"resourceContext,omitempty"`
}

// listAll fetches every page of a list operation and returns the items merged
// in server order, plus the first page's collection resourceContext. op names
// the operation in errors ("list projects").
func listAll[T any](ctx context.Context, c *httpclient.Client, op, path string, opts ListOptions) ([]T, *ResourceContext, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	items := []T{}
	var collection *ResourceContext
	fetched := 0
	for n := 1; ; n++ {
		if n > maxPages {
			return nil, nil, fmt.Errorf("%s: stopped after %d pages", op, maxPages)
		}
		req := c.HTTP().R().SetContext(ctx).
			SetQueryParam("page", strconv.Itoa(n)).
			SetQueryParam("page-size", strconv.Itoa(pageSize))
		if opts.Sort != "" {
			req.SetQueryParam("sort", opts.Sort)
		}
		for _, f := range opts.AddFields {
			req.QueryParam.Add("add-fields", f)
		}

		resp, err := req.Get(path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", op, err)
		}
		if err := httpclient.CheckResponse(resp); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", op, err)
		}
		var p page[T]
		if err := json.Unmarshal(resp.Body(), &p); err != nil {
			return nil, nil, fmt.Errorf("%s: parse response: %w", op, err)
		}

		if n == 1 {
			collection = p.ResourceContext
		}
		items = append(items, p.Items...)
		fetched += len(p.Items)

		if lastPage(p.Pagination, len(p.Items), fetched, pageSize) {
			return items, collection, nil
		}
	}
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

// getOne fetches and decodes a single resource. op names the operation in
// errors (`get project "checkout"`).
func getOne[T any](ctx context.Context, c *httpclient.Client, op, path string, addFields []string) (*T, error) {
	req := c.HTTP().R().SetContext(ctx)
	for _, f := range addFields {
		req.QueryParam.Add("add-fields", f)
	}
	resp, err := req.Get(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	var result T
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", op, err)
	}
	return &result, nil
}

// projectPath is the path of a project-scoped collection or resource. Every
// segment goes through PathSegment.
func projectPath(projectKey string, segments ...string) string {
	path := BasePath + "/projects/" + httpclient.PathSegment(projectKey)
	for _, s := range segments {
		path += "/" + s
	}
	return path
}

// putOne sends a replacing write and decodes the resource it returns. version
// goes in the optimistic-locking-version query parameter when set. op names
// the operation in errors.
func putOne[T any](ctx context.Context, c *httpclient.Client, op, path string, body any, version string) (*T, error) {
	req := c.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body)
	if version != "" {
		req.SetQueryParam("optimistic-locking-version", version)
	}
	resp, err := req.Put(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	var result T
	if len(resp.Body()) == 0 {
		return &result, nil
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", op, err)
	}
	return &result, nil
}
