// Package featuremanagement wraps the public Dynatrace Feature Management API
// (/platform/feature-management/v0.1): projects, release stages, features and
// their flags, targeting, context attributes, SDK keys and overrides.
//
// One package covers the whole surface, one file per resource, so handler
// methods are named after the API's operationIds (ListProjects, GetProject)
// rather than a bare List/Get that every resource would want.
//
// The types are hand-written for now, in the shape code generated from the
// public OpenAPI document would take: schema names as Go type names
// (ProjectDetails, ProjectList), the API's JSON field names, and pointers for
// optional members. Whether to generate them is an open question for the
// maintainers (docs/dev/FEATURE_MANAGEMENT_DESIGN.md); the hand-written subset
// is kept small so generated code can replace it without renaming anything a
// caller uses.
//
// No file I/O, no display logic: that lives in pkg/resources/featuremanagement.
package featuremanagement

import (
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// BasePath is the API gateway path of the public Feature Management API.
const BasePath = "/platform/feature-management/v0.1"

// DefaultPageSize is the page size the list methods request when the caller
// does not set one. The API's own default is 20 and its maximum 1000.
const DefaultPageSize = 100

// maxPages bounds a paginated listing, so a server that keeps answering full
// pages cannot keep a caller looping forever.
const maxPages = 10000

// Handler is the client for the Feature Management API.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a Feature Management handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// ModificationInfo records who created and last modified an entity, and when.
type ModificationInfo struct {
	CreatedBy        string    `json:"createdBy" yaml:"createdBy"`
	CreatedTime      time.Time `json:"createdTime" yaml:"createdTime"`
	LastModifiedBy   string    `json:"lastModifiedBy" yaml:"lastModifiedBy"`
	LastModifiedTime time.Time `json:"lastModifiedTime" yaml:"lastModifiedTime"`
}

// ResourceContext lists the operations the caller may perform on a resource
// or collection (write, delete, publish, reject, cancel, metadata-write).
// `read` is implied by the resource having been returned. It describes
// permissions only; the server stays the authority on every call.
type ResourceContext struct {
	Operations []string `json:"operations,omitempty" yaml:"operations,omitempty"`
}

// Pagination describes one page of a list response. Feature Management pages
// by number (page, page-size), not by page key.
type Pagination struct {
	Page       *int   `json:"page,omitempty" yaml:"page,omitempty"`
	PageSize   *int   `json:"pageSize,omitempty" yaml:"pageSize,omitempty"`
	TotalCount *int64 `json:"totalCount,omitempty" yaml:"totalCount,omitempty"`
}

// ListOptions are the query parameters every Feature Management list
// operation shares.
type ListOptions struct {
	// PageSize is the number of items requested per page (1 to 1000). Zero
	// means DefaultPageSize. The list methods fetch every page either way;
	// this only sets how many round trips that takes.
	PageSize int
	// Sort is a comma-separated list of fields, each optionally prefixed with
	// '-' for descending order, sent verbatim as the sort parameter.
	Sort string
	// AddFields requests optional response members, such as
	// "resourceContext".
	AddFields []string
}
