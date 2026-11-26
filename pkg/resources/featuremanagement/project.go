// Package featuremanagement holds the CLI handlers for Feature Management
// resources: display types with table tags, and key-or-name resolution. The
// API calls live in sdk/api/featuremanagement.
package featuremanagement

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// Re-exported SDK types that carry no table tags.
type (
	ProjectSettings       = sdkfm.ProjectSettings
	ChangeRequestSettings = sdkfm.ChangeRequestSettings
	ModificationInfo      = sdkfm.ModificationInfo
	ResourceContext       = sdkfm.ResourceContext
)

// Project is the CLI view of a project. Its JSON and YAML keys are the API's
// own, in the API's order, so `-o json|yaml` prints the API read model; the
// *Display fields exist only for the table.
type Project struct {
	Key                   string           `json:"key" yaml:"key" table:"KEY"`
	Name                  string           `json:"name" yaml:"name" table:"NAME"`
	Description           string           `json:"description" yaml:"description" table:"DESCRIPTION,wide"`
	Owner                 string           `json:"owner" yaml:"owner" table:"OWNER"`
	Maintainers           []string         `json:"maintainers" yaml:"maintainers" table:"MAINTAINERS,wide"`
	Settings              ProjectSettings  `json:"settings" yaml:"settings" table:"-"`
	ChangeRequestsDisplay string           `json:"-" yaml:"-" table:"CHANGE_REQUESTS"`
	ModificationInfo      ModificationInfo `json:"modificationInfo" yaml:"modificationInfo" table:"-"`
	LastModifiedDisplay   string           `json:"-" yaml:"-" table:"LAST_MODIFIED,wide"`
	ResourceContext       *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty" table:"-"`
	Version               int              `json:"version" yaml:"version" table:"VERSION,wide"`
}

// ChangeRequestsEnabled reports the project's settings.changeRequests.enabled,
// which the API defaults to false when absent.
func (p *Project) ChangeRequestsEnabled() bool {
	cr := p.Settings.ChangeRequests
	return cr != nil && cr.Enabled != nil && *cr.Enabled
}

// fromSDKProject converts the SDK read model and fills the display fields.
func fromSDKProject(s *sdkfm.ProjectDetails) Project {
	p := Project{
		Key:              s.Key,
		Name:             s.Name,
		Description:      s.Description,
		Owner:            s.Owner,
		Maintainers:      s.Maintainers,
		Settings:         s.Settings,
		ModificationInfo: s.ModificationInfo,
		ResourceContext:  s.ResourceContext,
		Version:          s.Version,
	}
	if p.Maintainers == nil {
		p.Maintainers = []string{}
	}
	p.ChangeRequestsDisplay = onOff(p.ChangeRequestsEnabled())
	if t := s.ModificationInfo.LastModifiedTime; !t.IsZero() {
		p.LastModifiedDisplay = t.UTC().Format("2006-01-02 15:04:05")
	}
	return p
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Handler is the CLI handler for Feature Management resources.
type Handler struct {
	sdk *sdkfm.Handler
}

// NewHandler creates a Feature Management handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{sdk: sdkfm.NewHandler(httpclient.Wrap(c.HTTP()))}
}

// ListProjects returns every project in the environment, sorted by key.
func (h *Handler) ListProjects() ([]Project, error) {
	list, err := h.sdk.ListProjects(context.Background(), sdkfm.ListOptions{Sort: "key"})
	if err != nil {
		return nil, err
	}
	projects := make([]Project, len(list.Items))
	for i := range list.Items {
		projects[i] = fromSDKProject(&list.Items[i])
	}
	// The server sorts too; sorting here keeps the order stable whatever a
	// server does with an unknown sort field.
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].Key < projects[j].Key })
	return projects, nil
}

// GetProject returns one project by key or, when no project has that key,
// by name. The key is tried first and costs one request; a name costs a
// listing, and only when the key misses. Resolution never dispatches on the
// argument's shape, since the key pattern also matches names and UUIDs: the
// server decides. It answers 404 for a well-formed key nobody has, and 400 for
// an argument that cannot be a key at all ("Checkout flows"), which can then
// only be a name.
func (h *Handler) GetProject(keyOrName string) (*Project, error) {
	s, err := h.sdk.GetProject(context.Background(), keyOrName)
	if err == nil {
		p := fromSDKProject(s)
		return &p, nil
	}
	if !isKeyMiss(err) {
		return nil, err
	}

	list, listErr := h.sdk.ListProjects(context.Background(), sdkfm.ListOptions{})
	if listErr != nil {
		// The name lookup is a fallback; the key's 404 is the error that
		// answers what the caller asked.
		return nil, err
	}
	var matches []sdkfm.ProjectDetails
	for _, item := range list.Items {
		if item.Name == keyOrName {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("project %q not found (no project has that key or name)", keyOrName)
	case 1:
		p := fromSDKProject(&matches[0])
		return &p, nil
	default:
		keys := make([]string, len(matches))
		for i, m := range matches {
			keys[i] = m.Key
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("%d projects are named %q; pass a key instead: %s", len(matches), keyOrName, strings.Join(keys, ", "))
	}
}

// isKeyMiss reports whether a lookup by key failed because the argument is
// not an existing key: a 404, or the 400 the API returns when the argument
// does not match the key pattern.
func isKeyMiss(err error) bool {
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusBadRequest
}
