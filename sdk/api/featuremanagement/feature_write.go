package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// A feature is read and sent back whole on every update, which replaces what
// it covers and clears what a request leaves out, so the feature types keep
// the members they do not declare (see unknownFields).

func (v *Variant) UnmarshalJSON(data []byte) error {
	type plain Variant
	u, err := decodeKeepingUnknown(data, (*plain)(v))
	v.unknown = u
	return err
}

func (v Variant) MarshalJSON() ([]byte, error) {
	type plain Variant
	return encodeKeepingUnknown(plain(v), v.unknown)
}

func (f *FeatureFlag) UnmarshalJSON(data []byte) error {
	type plain FeatureFlag
	u, err := decodeKeepingUnknown(data, (*plain)(f))
	f.unknown = u
	return err
}

func (f FeatureFlag) MarshalJSON() ([]byte, error) {
	type plain FeatureFlag
	return encodeKeepingUnknown(plain(f), f.unknown)
}

func (d *FeatureDetails) UnmarshalJSON(data []byte) error {
	type plain FeatureDetails
	u, err := decodeKeepingUnknown(data, (*plain)(d))
	d.unknown = u
	return err
}

func (d FeatureDetails) MarshalJSON() ([]byte, error) {
	type plain FeatureDetails
	return encodeKeepingUnknown(plain(d), d.unknown)
}

// InitialFlagCreate is the single flag a feature can be created with
// (schema InitialFlagCreate). Without Variants the service fills each
// variant's value from the flag type.
type InitialFlagCreate struct {
	Key      string         `json:"key" yaml:"key"`
	Type     FlagType       `json:"type" yaml:"type"`
	Variants map[string]any `json:"variants,omitempty" yaml:"variants,omitempty"`
}

// FeatureFlagCreate is a flag created with its feature (schema
// FeatureFlagCreate). Only Key and Type are required; the service defaults
// the name to the key and the values to the type's defaults.
type FeatureFlagCreate struct {
	Key         string         `json:"key" yaml:"key"`
	Name        string         `json:"name,omitempty" yaml:"name,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Type        FlagType       `json:"type" yaml:"type"`
	Variants    map[string]any `json:"variants,omitempty" yaml:"variants,omitempty"`
	Schema      map[string]any `json:"schema,omitempty" yaml:"schema,omitempty"`

	unknown unknownFields
}

func (f *FeatureFlagCreate) UnmarshalJSON(data []byte) error {
	type plain FeatureFlagCreate
	u, err := decodeKeepingUnknown(data, (*plain)(f))
	f.unknown = u
	return err
}

func (f FeatureFlagCreate) MarshalJSON() ([]byte, error) {
	type plain FeatureFlagCreate
	return encodeKeepingUnknown(plain(f), f.unknown)
}

// FeatureCreate is the body of a feature create (schema FeatureCreate). Give
// either Flag, one flag, or Flags, several with names and schemas.
type FeatureCreate struct {
	Key         string              `json:"key" yaml:"key"`
	Name        string              `json:"name" yaml:"name"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty"`
	Type        FeatureType         `json:"type,omitempty" yaml:"type,omitempty"`
	Flag        *InitialFlagCreate  `json:"flag,omitempty" yaml:"flag,omitempty"`
	Flags       []FeatureFlagCreate `json:"flags,omitempty" yaml:"flags,omitempty"`
	Variants    []Variant           `json:"variants" yaml:"variants"`
	Owner       string              `json:"owner,omitempty" yaml:"owner,omitempty"`
	Maintainers []string            `json:"maintainers,omitempty" yaml:"maintainers,omitempty"`

	unknown unknownFields
}

func (c *FeatureCreate) UnmarshalJSON(data []byte) error {
	type plain FeatureCreate
	u, err := decodeKeepingUnknown(data, (*plain)(c))
	c.unknown = u
	return err
}

func (c FeatureCreate) MarshalJSON() ([]byte, error) {
	type plain FeatureCreate
	return encodeKeepingUnknown(plain(c), c.unknown)
}

// FeatureUpdate is the body of a full feature update (schema FeatureUpdate).
// It replaces the variant and flag lists: a flag left out is deleted, and a
// flag's omitted schema is removed. Build it from the feature just read with
// NewFeatureUpdate, which carries every field the read returned.
type FeatureUpdate struct {
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description" yaml:"description"`
	Type        FeatureType   `json:"type" yaml:"type"`
	Variants    []Variant     `json:"variants" yaml:"variants"`
	Flags       []FeatureFlag `json:"flags" yaml:"flags"`
	Owner       string        `json:"owner" yaml:"owner"`
	Maintainers []string      `json:"maintainers" yaml:"maintainers"`
	Status      FeatureStatus `json:"status" yaml:"status"`

	unknown unknownFields
}

func (u FeatureUpdate) MarshalJSON() ([]byte, error) {
	type plain FeatureUpdate
	return encodeKeepingUnknown(plain(u), u.unknown)
}

// NewFeatureUpdate is the update that writes d back unchanged, members this
// package does not declare included.
func NewFeatureUpdate(d *FeatureDetails) FeatureUpdate {
	variants := make([]Variant, len(d.Variants))
	for i, v := range d.Variants {
		v.ResourceContext = nil
		variants[i] = v
	}
	flags := append([]FeatureFlag{}, d.Flags...)
	maintainers := append([]string{}, d.Maintainers...)
	return FeatureUpdate{
		Name: d.Name, Description: d.Description, Type: d.Type, Variants: variants, Flags: flags,
		Owner: d.Owner, Maintainers: maintainers, Status: d.Status, unknown: d.unknown,
	}
}

// VariantMetadataUpdate is a variant's descriptive fields.
type VariantMetadataUpdate struct {
	Key          string `json:"key" yaml:"key"`
	Name         string `json:"name" yaml:"name"`
	DisplayOrder *int   `json:"displayOrder,omitempty" yaml:"displayOrder,omitempty"`
}

// FlagMetadataUpdate is a flag's descriptive fields.
type FlagMetadataUpdate struct {
	Key         string `json:"key" yaml:"key"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

// FeatureMetadataUpdate is the body of a descriptive update (schema
// FeatureMetadataUpdate, scope features.metadata:write). It cannot add or
// remove variants or flags, and it clears omitted optional fields, so build it
// from the feature just read with NewFeatureMetadataUpdate.
type FeatureMetadataUpdate struct {
	Name        string                  `json:"name" yaml:"name"`
	Description string                  `json:"description" yaml:"description"`
	Type        FeatureType             `json:"type" yaml:"type"`
	Variants    []VariantMetadataUpdate `json:"variants" yaml:"variants"`
	Flags       []FlagMetadataUpdate    `json:"flags" yaml:"flags"`
	Owner       string                  `json:"owner" yaml:"owner"`
	Maintainers []string                `json:"maintainers" yaml:"maintainers"`

	unknown unknownFields
}

func (u FeatureMetadataUpdate) MarshalJSON() ([]byte, error) {
	type plain FeatureMetadataUpdate
	return encodeKeepingUnknown(plain(u), u.unknown)
}

// NewFeatureMetadataUpdate is the descriptive update that writes d's
// descriptive fields back unchanged, top-level members this package does not
// declare included.
func NewFeatureMetadataUpdate(d *FeatureDetails) FeatureMetadataUpdate {
	u := FeatureMetadataUpdate{
		Name: d.Name, Description: d.Description, Type: d.Type,
		Owner: d.Owner, Maintainers: append([]string{}, d.Maintainers...), unknown: d.unknown,
		Variants: make([]VariantMetadataUpdate, len(d.Variants)),
		Flags:    make([]FlagMetadataUpdate, len(d.Flags)),
	}
	for i, v := range d.Variants {
		u.Variants[i] = VariantMetadataUpdate{Key: v.Key, Name: v.Name, DisplayOrder: v.DisplayOrder}
	}
	for i, f := range d.Flags {
		u.Flags[i] = FlagMetadataUpdate{Key: f.Key, Name: f.Name, Description: f.Description}
	}
	return u
}

// FeaturePatch is the body of a status change (schema FeaturePatch).
type FeaturePatch struct {
	Status FeatureStatus `json:"status" yaml:"status"`
}

func featurePath(projectKey, featureKey string) string {
	return projectPath(projectKey, "features", httpclient.PathSegment(featureKey))
}

// CreateFeature creates a feature (createFeature).
func (h *Handler) CreateFeature(ctx context.Context, projectKey string, body FeatureCreate) (*FeatureDetails, error) {
	return postOne[FeatureDetails](ctx, h.client,
		fmt.Sprintf("create feature %q in project %q", body.Key, projectKey), projectPath(projectKey, "features"), body)
}

// UpdateFeature replaces a feature's variants, flags and fields
// (updateFeature). version is the optimistic-locking version read with it.
func (h *Handler) UpdateFeature(ctx context.Context, projectKey, featureKey string, body FeatureUpdate, version string) (*FeatureDetails, error) {
	return putOne[FeatureDetails](ctx, h.client,
		fmt.Sprintf("update feature %q of project %q", featureKey, projectKey), featurePath(projectKey, featureKey), body, version)
}

// UpdateFeatureMetadata replaces a feature's descriptive fields
// (updateFeatureMetadata).
func (h *Handler) UpdateFeatureMetadata(ctx context.Context, projectKey, featureKey string, body FeatureMetadataUpdate, version string) (*OptimisticLockingVersionResponse, error) {
	return putOne[OptimisticLockingVersionResponse](ctx, h.client,
		fmt.Sprintf("update metadata of feature %q of project %q", featureKey, projectKey),
		featurePath(projectKey, featureKey)+"/metadata", body, version)
}

// PatchFeature changes a feature's status (patchFeature).
func (h *Handler) PatchFeature(ctx context.Context, projectKey, featureKey string, body FeaturePatch, version string) (*OptimisticLockingVersionResponse, error) {
	return send[OptimisticLockingVersionResponse](ctx, h.client,
		fmt.Sprintf("change status of feature %q of project %q", featureKey, projectKey),
		"PATCH", featurePath(projectKey, featureKey), body, version)
}

// DeleteFeature deletes a feature with its flags and targeting
// (deleteFeature).
func (h *Handler) DeleteFeature(ctx context.Context, projectKey, featureKey string) error {
	return deleteOne(ctx, h.client, fmt.Sprintf("delete feature %q of project %q", featureKey, projectKey), featurePath(projectKey, featureKey))
}
