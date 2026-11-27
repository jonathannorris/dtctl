package featuremanagement

import (
	"context"
	"fmt"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// ReleaseStageCreate re-exports the SDK's release stage create body.
type ReleaseStageCreate = sdkfm.ReleaseStageCreate

// ReleaseStageFile reads a release stage from a create file: the API's
// ReleaseStageCreate fields, or a stage exported with get -o yaml.
func ReleaseStageFile(projectKey string) ResourceFile {
	return ResourceFile{
		Kind:     "fm/release-stage",
		ReadOnly: []string{"modificationInfo", "resourceContext", "version"},
		Parents:  map[string]string{"projectKey": projectKey},
	}
}

// ReleaseStageWriteResult is the outcome of a release stage write.
type ReleaseStageWriteResult struct {
	// Stage is the stage after the write, or as read when nothing changed.
	Stage *ReleaseStage
	// Changed is false when the stage already was what the command asked
	// for, so nothing was sent.
	Changed bool
}

// CreateReleaseStage creates a release stage.
func (h *Handler) CreateReleaseStage(projectKey string, body ReleaseStageCreate) (*ReleaseStageWriteResult, error) {
	d, err := h.sdk.CreateReleaseStage(context.Background(), projectKey, body)
	if err != nil {
		return nil, err
	}
	r := fromSDKReleaseStage(d)
	return &ReleaseStageWriteResult{Stage: &r, Changed: true}, nil
}

// ReleaseStageChange is a release stage update; nil fields are kept.
type ReleaseStageChange struct {
	Name, Description *string
	Type              *ReleaseStageType
	Guarded           *bool
	DisplayOrder      *int
}

// UpdateReleaseStage changes a release stage's fields. It sends back
// everything it read, with the version it read.
func (h *Handler) UpdateReleaseStage(projectKey, keyOrName string, c ReleaseStageChange, opts WriteOptions) (*ReleaseStageWriteResult, error) {
	r, err := h.GetReleaseStage(projectKey, keyOrName)
	if err != nil {
		return nil, err
	}
	d, err := h.sdk.GetReleaseStage(context.Background(), projectKey, r.Key)
	if err != nil {
		return nil, err
	}
	body := sdkfm.NewReleaseStageUpdate(d)
	before := sdkfm.NewReleaseStageUpdate(d)
	if c.Name != nil {
		body.Name = *c.Name
	}
	if c.Description != nil {
		body.Description = *c.Description
	}
	if c.Type != nil {
		body.Type = *c.Type
	}
	if c.Guarded != nil {
		g := *c.Guarded
		body.Guarded = &g
	}
	if c.DisplayOrder != nil {
		body.DisplayOrder = *c.DisplayOrder
	}
	if sameJSON(before, body) {
		s := fromSDKReleaseStage(d)
		return &ReleaseStageWriteResult{Stage: &s}, nil
	}
	updated, err := h.sdk.UpdateReleaseStage(context.Background(), projectKey, d.Key, body, opts.version(d.Version))
	if err != nil {
		return nil, conflictError(err, fmt.Sprintf("release stage %q", d.Key))
	}
	s := fromSDKReleaseStage(updated)
	return &ReleaseStageWriteResult{Stage: &s, Changed: true}, nil
}

// ReorderReleaseStages sets the project's release stage order: keys names
// every stage, and each gets its position as its display order. Display
// order ranks stages within their environment type, so the list orders the
// stages of each type among themselves. Stages already in that order are
// left alone and Changed is false.
func (h *Handler) ReorderReleaseStages(projectKey string, keys []string) ([]ReleaseStage, bool, error) {
	stages, err := h.ListReleaseStages(projectKey)
	if err != nil {
		return nil, false, err
	}
	byKey := map[string]ReleaseStage{}
	for _, s := range stages {
		byKey[s.Key] = s
	}
	body := sdkfm.ReleaseStageReorder{}
	seen := map[string]bool{}
	changed := false
	for i, k := range keys {
		k = strings.TrimSpace(k)
		s, ok := byKey[k]
		switch {
		case !ok:
			return nil, false, fmt.Errorf("--order: project %s has no release stage %q (stages: %s)", projectKey, k, releaseStageKeys(stages))
		case seen[k]:
			return nil, false, fmt.Errorf("--order: release stage %q is listed twice", k)
		}
		seen[k] = true
		if s.DisplayOrder != i {
			changed = true
		}
		body.Items = append(body.Items, sdkfm.ReleaseStageOrder{Key: k, DisplayOrder: i, Version: s.Version})
	}
	if len(seen) != len(stages) {
		var missing []string
		for _, s := range stages {
			if !seen[s.Key] {
				missing = append(missing, s.Key)
			}
		}
		return nil, false, fmt.Errorf("--order must name every release stage; missing: %s", strings.Join(missing, ", "))
	}
	if !changed {
		return stages, false, nil
	}
	if _, err := h.sdk.ReorderReleaseStages(context.Background(), projectKey, body); err != nil {
		return nil, false, conflictError(err, "a release stage")
	}
	stages, err = h.ListReleaseStages(projectKey)
	return stages, true, err
}

func releaseStageKeys(stages []ReleaseStage) string {
	keys := make([]string, len(stages))
	for i, s := range stages {
		keys[i] = s.Key
	}
	return strings.Join(keys, ", ")
}

// DeleteReleaseStage deletes a release stage, its SDK keys and every
// feature's targeting in it, and returns the key it deleted.
func (h *Handler) DeleteReleaseStage(projectKey, keyOrName string) (string, error) {
	r, err := h.GetReleaseStage(projectKey, keyOrName)
	if err != nil {
		return "", err
	}
	return r.Key, h.sdk.DeleteReleaseStage(context.Background(), projectKey, r.Key)
}
