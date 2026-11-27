// Package featuremanagement holds the CLI handlers for Feature Management
// resources: display types with table tags, and key-or-name resolution. The
// API calls live in sdk/api/featuremanagement.
package featuremanagement

import (
	"context"
	"sort"

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
	p.LastModifiedDisplay = displayTime(s.ModificationInfo.LastModifiedTime)
	return p
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
// by name (see resolveKeyOrName).
func (h *Handler) GetProject(keyOrName string) (*Project, error) {
	p, err := resolveKeyOrName("project", keyOrName,
		func(key string) (Project, error) {
			s, err := h.sdk.GetProject(context.Background(), key)
			if err != nil {
				return Project{}, err
			}
			return fromSDKProject(s), nil
		},
		func() ([]Project, error) {
			list, err := h.sdk.ListProjects(context.Background(), sdkfm.ListOptions{})
			if err != nil {
				return nil, err
			}
			out := make([]Project, len(list.Items))
			for i := range list.Items {
				out[i] = fromSDKProject(&list.Items[i])
			}
			return out, nil
		},
		func(p Project) string { return p.Key },
		func(p Project) string { return p.Name },
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
