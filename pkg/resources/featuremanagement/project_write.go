package featuremanagement

import (
	"context"
	"fmt"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// ProjectCreate re-exports the SDK's project create body.
type ProjectCreate = sdkfm.ProjectCreate

// ProjectFile reads a project from a create file: the API's ProjectCreate
// fields, or a project exported with get -o yaml.
func ProjectFile() ResourceFile {
	return ResourceFile{Kind: "fm/project", ReadOnly: []string{"modificationInfo", "resourceContext", "version"}}
}

// ProjectWriteResult is the outcome of a project write.
type ProjectWriteResult struct {
	// Project is the project after the write, or as read when nothing
	// changed.
	Project *Project
	// Changed is false when the project already was what the command asked
	// for, so nothing was sent.
	Changed bool
}

// CreateProject creates a project.
func (h *Handler) CreateProject(body ProjectCreate) (*ProjectWriteResult, error) {
	d, err := h.sdk.CreateProject(context.Background(), body)
	if err != nil {
		return nil, err
	}
	p := fromSDKProject(d)
	return &ProjectWriteResult{Project: &p, Changed: true}, nil
}

// readProject reads a project by key or name with every member the API
// returned, for a read-modify-write.
func (h *Handler) readProject(keyOrName string) (*sdkfm.ProjectDetails, error) {
	p, err := h.GetProject(keyOrName)
	if err != nil {
		return nil, err
	}
	return h.sdk.GetProject(context.Background(), p.Key)
}

// ProjectChange is a project update; nil fields are kept.
type ProjectChange struct {
	Name, Description, Owner *string
	Maintainers              *[]string
	ChangeRequests           *bool
}

// UpdateProject changes a project's fields. It sends back everything it
// read, settings this dtctl doesn't know included, with the version it read.
func (h *Handler) UpdateProject(keyOrName string, c ProjectChange, opts WriteOptions) (*ProjectWriteResult, error) {
	d, err := h.readProject(keyOrName)
	if err != nil {
		return nil, err
	}
	body := sdkfm.NewProjectUpdate(d)
	before := sdkfm.NewProjectUpdate(d)
	if c.Name != nil {
		body.Name = *c.Name
	}
	if c.Description != nil {
		body.Description = *c.Description
	}
	if c.Owner != nil {
		body.Owner = *c.Owner
	}
	if c.Maintainers != nil {
		body.Maintainers = append([]string{}, *c.Maintainers...)
	}
	if c.ChangeRequests != nil {
		cr := sdkfm.ChangeRequestSettings{}
		if body.Settings.ChangeRequests != nil {
			cr = *body.Settings.ChangeRequests
		}
		enabled := *c.ChangeRequests
		cr.Enabled = &enabled
		body.Settings.ChangeRequests = &cr
	}
	if sameJSON(before, body) {
		p := fromSDKProject(d)
		return &ProjectWriteResult{Project: &p}, nil
	}
	updated, err := h.sdk.UpdateProject(context.Background(), d.Key, body, opts.version(d.Version))
	if err != nil {
		return nil, conflictError(err, fmt.Sprintf("project %q", d.Key))
	}
	p := fromSDKProject(updated)
	return &ProjectWriteResult{Project: &p, Changed: true}, nil
}

// DeleteProject deletes a project with everything in it and returns the key
// it deleted.
func (h *Handler) DeleteProject(keyOrName string) (string, error) {
	p, err := h.GetProject(keyOrName)
	if err != nil {
		return "", err
	}
	return p.Key, h.sdk.DeleteProject(context.Background(), p.Key)
}
