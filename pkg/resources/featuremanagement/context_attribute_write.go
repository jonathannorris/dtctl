package featuremanagement

import (
	"context"
	"fmt"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// ContextAttributeCreate re-exports the SDK's context attribute create body.
type ContextAttributeCreate = sdkfm.ContextAttributeCreate

// ContextAttributeFile reads a context attribute from a create file: the
// API's ContextAttributeCreate fields, or an attribute exported with
// get -o yaml, whose read-only members are dropped.
func ContextAttributeFile(projectKey string) ResourceFile {
	return ResourceFile{
		Kind:     "fm/context-attribute",
		ReadOnly: []string{"modificationInfo", "resourceContext", "version", "usageCount", "builtIn"},
		Parents:  map[string]string{"projectKey": projectKey},
	}
}

// ContextAttributeWriteResult is the outcome of a context attribute write.
type ContextAttributeWriteResult struct {
	// Attribute is the attribute after the write, or as read when nothing
	// changed.
	Attribute *ContextAttribute
	// Changed is false when the attribute already was what the command
	// asked for, so nothing was sent.
	Changed bool
}

// CreateContextAttribute creates a context attribute.
func (h *Handler) CreateContextAttribute(projectKey string, body ContextAttributeCreate) (*ContextAttributeWriteResult, error) {
	d, err := h.sdk.CreateContextAttribute(context.Background(), projectKey, body)
	if err != nil {
		return nil, err
	}
	a := fromSDKContextAttribute(d)
	return &ContextAttributeWriteResult{Attribute: &a, Changed: true}, nil
}

// ContextAttributeChange is a context attribute update; nil fields are
// kept. An attribute's type and context key cannot change.
type ContextAttributeChange struct {
	Name, Description, Schema *string
}

// UpdateContextAttribute changes a context attribute's name, description or
// schema, with the version it read.
func (h *Handler) UpdateContextAttribute(projectKey, keyOrName string, c ContextAttributeChange, opts WriteOptions) (*ContextAttributeWriteResult, error) {
	a, err := h.GetContextAttribute(projectKey, keyOrName)
	if err != nil {
		return nil, err
	}
	d, err := h.sdk.GetContextAttribute(context.Background(), projectKey, a.Key)
	if err != nil {
		return nil, err
	}
	body := sdkfm.NewContextAttributeUpdate(d)
	before := sdkfm.NewContextAttributeUpdate(d)
	if c.Name != nil {
		body.Name = *c.Name
	}
	if c.Description != nil {
		body.Description = *c.Description
	}
	if c.Schema != nil {
		body.Schema = *c.Schema
	}
	if sameJSON(before, body) {
		out := fromSDKContextAttribute(d)
		return &ContextAttributeWriteResult{Attribute: &out}, nil
	}
	updated, err := h.sdk.UpdateContextAttribute(context.Background(), projectKey, d.Key, body, opts.version(d.Version))
	if err != nil {
		return nil, conflictError(err, fmt.Sprintf("context attribute %q", d.Key))
	}
	out := fromSDKContextAttribute(updated)
	return &ContextAttributeWriteResult{Attribute: &out, Changed: true}, nil
}

// DeleteContextAttribute deletes a context attribute and returns the key it
// deleted. An attribute a targeting rule still references is refused before
// anything is sent, naming the features whose rules do.
func (h *Handler) DeleteContextAttribute(projectKey, keyOrName string) (string, error) {
	a, err := h.GetContextAttribute(projectKey, keyOrName)
	if err != nil {
		return "", err
	}
	usage, err := h.GetContextAttributeUsage(projectKey, a.Key)
	if err != nil {
		return "", err
	}
	if len(usage) > 0 {
		features := make([]string, len(usage))
		for i, u := range usage {
			features[i] = u.FeatureKey
		}
		return "", fmt.Errorf("context attribute %s is still used by the targeting of %s; remove those conditions first (see 'dtctl describe fm context-attribute %s')",
			a.Key, strings.Join(features, ", "), a.Key)
	}
	return a.Key, h.sdk.DeleteContextAttribute(context.Background(), projectKey, a.Key)
}
