package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// Context attributes are read and sent back on update, so the read model
// keeps the members it does not declare (see unknownFields).

func (v *ContextAttributeDetails) UnmarshalJSON(data []byte) error {
	type plain ContextAttributeDetails
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ContextAttributeDetails) MarshalJSON() ([]byte, error) {
	type plain ContextAttributeDetails
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ContextAttributeCreate is the body of a context attribute create (schema
// ContextAttributeCreate). Its type and context key cannot change later.
type ContextAttributeCreate struct {
	Key         string               `json:"key" yaml:"key"`
	Name        string               `json:"name" yaml:"name"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Schema      string               `json:"schema" yaml:"schema"`
	Type        ContextAttributeType `json:"type" yaml:"type"`
	ContextKey  string               `json:"contextKey" yaml:"contextKey"`

	unknown unknownFields
}

func (v *ContextAttributeCreate) UnmarshalJSON(data []byte) error {
	type plain ContextAttributeCreate
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v ContextAttributeCreate) MarshalJSON() ([]byte, error) {
	type plain ContextAttributeCreate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// ContextAttributeUpdate is the body of a context attribute update (schema
// ContextAttributeUpdate): its name, description and schema. Build it from
// the attribute just read with NewContextAttributeUpdate.
type ContextAttributeUpdate struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Schema      string `json:"schema" yaml:"schema"`

	unknown unknownFields
}

func (v ContextAttributeUpdate) MarshalJSON() ([]byte, error) {
	type plain ContextAttributeUpdate
	return encodeKeepingUnknown(plain(v), v.unknown)
}

// contextAttributeReadOnly are the read model's members an update does not
// take: identity, type, usage and bookkeeping.
var contextAttributeReadOnly = []string{"builtIn", "contextKey", "projectKey", "type", "usageCount"}

// NewContextAttributeUpdate is the update that writes d back unchanged.
func NewContextAttributeUpdate(d *ContextAttributeDetails) ContextAttributeUpdate {
	u := ContextAttributeUpdate{Name: d.Name, Description: d.Description, Schema: d.Schema}
	if len(d.unknown) > 0 {
		u.unknown = unknownFields{}
		for k, v := range d.unknown {
			u.unknown[k] = v
		}
		for _, k := range contextAttributeReadOnly {
			delete(u.unknown, k)
		}
	}
	return u
}

func contextAttributePath(projectKey, key string) string {
	return projectPath(projectKey, "context-attributes", httpclient.PathSegment(key))
}

// CreateContextAttribute creates a context attribute (createContextAttribute).
func (h *Handler) CreateContextAttribute(ctx context.Context, projectKey string, body ContextAttributeCreate) (*ContextAttributeDetails, error) {
	return postOne[ContextAttributeDetails](ctx, h.client,
		fmt.Sprintf("create context attribute %q in project %q", body.Key, projectKey), projectPath(projectKey, "context-attributes"), body)
}

// UpdateContextAttribute replaces a context attribute's name, description
// and schema (updateContextAttribute).
func (h *Handler) UpdateContextAttribute(ctx context.Context, projectKey, key string, body ContextAttributeUpdate, version string) (*ContextAttributeDetails, error) {
	return putOne[ContextAttributeDetails](ctx, h.client,
		fmt.Sprintf("update context attribute %q of project %q", key, projectKey), contextAttributePath(projectKey, key), body, version)
}

// DeleteContextAttribute deletes a context attribute (deleteContextAttribute).
func (h *Handler) DeleteContextAttribute(ctx context.Context, projectKey, key string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete context attribute %q of project %q", key, projectKey), contextAttributePath(projectKey, key))
}
