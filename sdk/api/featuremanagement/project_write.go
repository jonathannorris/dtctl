package featuremanagement

import (
	"context"
	"fmt"
)

// Projects are read and sent back whole on update, so the project types keep
// the members they do not declare (see unknownFields).

func (v *ChangeRequestSettings) UnmarshalJSON(data []byte) error {
	type plain ChangeRequestSettings
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ChangeRequestSettings) MarshalJSON() ([]byte, error) {
	type plain ChangeRequestSettings
	return encodeKeepingUnknown(plain(v), v.unknown)
}

func (v *ProjectSettings) UnmarshalJSON(data []byte) error {
	type plain ProjectSettings
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ProjectSettings) MarshalJSON() ([]byte, error) {
	type plain ProjectSettings
	return encodeKeepingUnknown(plain(v), v.unknown)
}

func (v *ProjectDetails) UnmarshalJSON(data []byte) error {
	type plain ProjectDetails
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ProjectDetails) MarshalJSON() ([]byte, error) {
	type plain ProjectDetails
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ProjectCreate is the body of a project create (schema ProjectCreate).
type ProjectCreate struct {
	Key         string           `json:"key" yaml:"key"`
	Name        string           `json:"name" yaml:"name"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
	Owner       string           `json:"owner,omitempty" yaml:"owner,omitempty"`
	Maintainers []string         `json:"maintainers,omitempty" yaml:"maintainers,omitempty"`
	Settings    *ProjectSettings `json:"settings,omitempty" yaml:"settings,omitempty"`

	unknown unknownFields
}

func (v *ProjectCreate) UnmarshalJSON(data []byte) error {
	type plain ProjectCreate
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ProjectCreate) MarshalJSON() ([]byte, error) {
	type plain ProjectCreate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ProjectUpdate is the body of a project update (schema ProjectUpdate). It
// replaces the project's fields, so build it from the project just read with
// NewProjectUpdate.
type ProjectUpdate struct {
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description" yaml:"description"`
	Owner       string          `json:"owner" yaml:"owner"`
	Maintainers []string        `json:"maintainers" yaml:"maintainers"`
	Settings    ProjectSettings `json:"settings" yaml:"settings"`

	unknown unknownFields
}

func (v ProjectUpdate) MarshalJSON() ([]byte, error) {
	type plain ProjectUpdate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// NewProjectUpdate is the update that writes d back unchanged.
func NewProjectUpdate(d *ProjectDetails) ProjectUpdate {
	return ProjectUpdate{
		Name: d.Name, Description: d.Description, Owner: d.Owner,
		Maintainers: append([]string{}, d.Maintainers...), Settings: d.Settings, unknown: d.unknown,
	}
}

// CreateProject creates a project (createProject).
func (h *Handler) CreateProject(ctx context.Context, body ProjectCreate) (*ProjectDetails, error) {
	return postOne[ProjectDetails](ctx, h.client, fmt.Sprintf("create project %q", body.Key), BasePath+"/projects", body)
}

// UpdateProject replaces a project's fields (updateProject).
func (h *Handler) UpdateProject(ctx context.Context, projectKey string, body ProjectUpdate, version string) (*ProjectDetails, error) {
	return putOne[ProjectDetails](ctx, h.client, fmt.Sprintf("update project %q", projectKey), projectPath(projectKey), body, version)
}

// DeleteProject deletes a project with every feature, release stage,
// context attribute and SDK key in it (deleteProject).
func (h *Handler) DeleteProject(ctx context.Context, projectKey string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete project %q", projectKey), projectPath(projectKey))
}
