package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// Release stages are read and sent back whole on update, so the read model
// keeps the members it does not declare (see unknownFields).

func (v *ReleaseStageDetails) UnmarshalJSON(data []byte) error {
	type plain ReleaseStageDetails
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ReleaseStageDetails) MarshalJSON() ([]byte, error) {
	type plain ReleaseStageDetails
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ReleaseStageCreate is the body of a release stage create (schema
// ReleaseStageCreate). Without DisplayOrder the stage goes first in its
// environment type.
type ReleaseStageCreate struct {
	Key          string           `json:"key" yaml:"key"`
	Name         string           `json:"name" yaml:"name"`
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	Type         ReleaseStageType `json:"type" yaml:"type"`
	Guarded      *bool            `json:"guarded,omitempty" yaml:"guarded,omitempty"`
	DisplayOrder *int             `json:"displayOrder,omitempty" yaml:"displayOrder,omitempty"`

	unknown unknownFields
}

func (v *ReleaseStageCreate) UnmarshalJSON(data []byte) error {
	type plain ReleaseStageCreate
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ReleaseStageCreate) MarshalJSON() ([]byte, error) {
	type plain ReleaseStageCreate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ReleaseStageUpdate is the body of a release stage update (schema
// ReleaseStageUpdate). Build it from the stage just read with
// NewReleaseStageUpdate.
type ReleaseStageUpdate struct {
	Name         string           `json:"name" yaml:"name"`
	Description  string           `json:"description" yaml:"description"`
	Type         ReleaseStageType `json:"type" yaml:"type"`
	Guarded      *bool            `json:"guarded,omitempty" yaml:"guarded,omitempty"`
	DisplayOrder int              `json:"displayOrder" yaml:"displayOrder"`

	unknown unknownFields
}

func (v ReleaseStageUpdate) MarshalJSON() ([]byte, error) {
	type plain ReleaseStageUpdate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// NewReleaseStageUpdate is the update that writes d back unchanged.
func NewReleaseStageUpdate(d *ReleaseStageDetails) ReleaseStageUpdate {
	return ReleaseStageUpdate{
		Name: d.Name, Description: d.Description, Type: d.Type, Guarded: d.Guarded,
		DisplayOrder: d.DisplayOrder, unknown: d.unknown,
	}
}

// ReleaseStageOrder is one stage's place in a reorder: its display order
// within its environment type, and the version it was read at.
type ReleaseStageOrder struct {
	Key          string `json:"key" yaml:"key"`
	DisplayOrder int    `json:"displayOrder" yaml:"displayOrder"`
	Version      int    `json:"version" yaml:"version"`
}

// ReleaseStageReorder is the body of a reorder (schema ReleaseStageReorder).
type ReleaseStageReorder struct {
	Items []ReleaseStageOrder `json:"items" yaml:"items"`
}

func releaseStagePath(projectKey, key string) string {
	return projectPath(projectKey, "release-stages", httpclient.PathSegment(key))
}

// CreateReleaseStage creates a release stage (createReleaseStage).
func (h *Handler) CreateReleaseStage(ctx context.Context, projectKey string, body ReleaseStageCreate) (*ReleaseStageDetails, error) {
	return postOne[ReleaseStageDetails](ctx, h.client,
		fmt.Sprintf("create release stage %q in project %q", body.Key, projectKey), projectPath(projectKey, "release-stages"), body)
}

// UpdateReleaseStage replaces a release stage's fields (updateReleaseStage).
func (h *Handler) UpdateReleaseStage(ctx context.Context, projectKey, key string, body ReleaseStageUpdate, version string) (*ReleaseStageDetails, error) {
	return putOne[ReleaseStageDetails](ctx, h.client,
		fmt.Sprintf("update release stage %q of project %q", key, projectKey), releaseStagePath(projectKey, key), body, version)
}

// ReorderReleaseStages sets every stage's display order in one write
// (reorderReleaseStages). Each item carries the version it was read at.
func (h *Handler) ReorderReleaseStages(ctx context.Context, projectKey string, body ReleaseStageReorder) (*ReleaseStageList, error) {
	return putOne[ReleaseStageList](ctx, h.client,
		fmt.Sprintf("reorder release stages of project %q", projectKey), projectPath(projectKey, "release-stages", "reorder"), body, "")
}

// DeleteReleaseStage deletes a release stage, revoking its SDK keys and
// dropping every feature's targeting in it (deleteReleaseStage).
func (h *Handler) DeleteReleaseStage(ctx context.Context, projectKey, key string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete release stage %q of project %q", key, projectKey), releaseStagePath(projectKey, key))
}
