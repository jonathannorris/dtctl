package featuremanagement

import (
	"context"
	"fmt"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// SDKKeyCreateResult is a created SDK key, in full: it is created to be
// used, so the caller needs the secret.
type SDKKeyCreateResult struct {
	Key *SDKKey
	// Existing is how many keys of the same type the stage already had,
	// which a caller may have meant to reuse.
	Existing int
}

// CreateSDKKey creates an SDK key of type t in a release stage, given by key
// or name.
func (h *Handler) CreateSDKKey(projectKey, releaseStageKeyOrName string, t SDKKeyType) (*SDKKeyCreateResult, error) {
	stage, err := h.GetReleaseStage(projectKey, releaseStageKeyOrName)
	if err != nil {
		return nil, err
	}
	existing, err := h.ListSDKKeys(projectKey, stage.Key, SDKKeyListOptions{Type: t})
	if err != nil {
		return nil, err
	}
	d, err := h.sdk.CreateSDKKey(context.Background(), projectKey, stage.Key, sdkfm.SDKKeyCreate{Type: t})
	if err != nil {
		return nil, err
	}
	k := fromSDKSDKKey(stage.Key, d, true)
	return &SDKKeyCreateResult{Key: &k, Existing: len(existing)}, nil
}

// DeleteSDKKey revokes an SDK key of a release stage. The key is given in
// full, masked (as get fm sdk-keys prints it) or as its public identifier;
// it returns the key masked.
func (h *Handler) DeleteSDKKey(projectKey, releaseStageKeyOrName, sdkKey string) (string, error) {
	stage, err := h.GetReleaseStage(projectKey, releaseStageKeyOrName)
	if err != nil {
		return "", err
	}
	keys, err := h.ListSDKKeys(projectKey, stage.Key, SDKKeyListOptions{Reveal: true})
	if err != nil {
		return "", err
	}
	want := strings.TrimSpace(sdkKey)
	id := sdkfm.SDKKeyPublicID(want)
	if id == "" {
		id = want // a public identifier alone
	}
	for _, k := range keys {
		if k.SDKKey == want || sdkfm.SDKKeyPublicID(k.SDKKey) == id {
			masked := sdkfm.MaskSDKKey(k.SDKKey)
			return masked, h.sdk.DeleteSDKKey(context.Background(), projectKey, stage.Key, k.SDKKey)
		}
	}
	return "", fmt.Errorf("release stage %s has no SDK key %s; list them with 'dtctl get fm sdk-keys %s'", stage.Key, MaskSDKKeyArg(want), stage.Key)
}

// MaskSDKKeyArg shows an SDK key a caller typed without its secret: masked
// when it has one, else as its public identifier, and never whole.
func MaskSDKKeyArg(arg string) string {
	if m := sdkfm.MaskSDKKey(arg); m != "" {
		return m
	}
	if m := sdkfm.MaskSDKKey(arg + ".x"); m != "" {
		return m
	}
	return redactedSDKKey
}
