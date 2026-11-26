package featuremanagement

import (
	"context"
	"sort"
	"time"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// SDKKeyType re-exports the SDK's SDK key type.
type SDKKeyType = sdkfm.SDKKeyType

// SDKKey is the CLI view of an SDK key. SDKKey is masked by default in every
// output format, keeping only the public, log-safe identifier
// (dt01.server_us_AbCd123456.********), so a routine listing cannot leak a
// live key into a terminal, a log or an agent transcript. With Reveal it is
// the full value, so a caller can reuse an existing key instead of creating
// one (design decision 7).
type SDKKey struct {
	ReleaseStageKey  string            `json:"releaseStageKey" yaml:"releaseStageKey" table:"-"`
	SDKKey           string            `json:"sdkKey" yaml:"sdkKey" table:"SDK_KEY"`
	Type             SDKKeyType        `json:"type" yaml:"type" table:"TYPE"`
	ModificationInfo *ModificationInfo `json:"modificationInfo,omitempty" yaml:"modificationInfo,omitempty" table:"-"`
	CreatedDisplay   string            `json:"-" yaml:"-" table:"CREATED"`
	CreatedByDisplay string            `json:"-" yaml:"-" table:"CREATED_BY,wide"`
	ResourceContext  *ResourceContext  `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty" table:"-"`
	Version          *int              `json:"version,omitempty" yaml:"version,omitempty" table:"-"`
}

// redactedSDKKey stands in for a masked key whose value has an unexpected
// shape, so such a key is never printed whole.
const redactedSDKKey = "(unrecognized key format)"

// SDKKeyListOptions selects and shapes the keys ListSDKKeys returns.
type SDKKeyListOptions struct {
	// Type keeps only keys of this type; empty keeps every type.
	Type SDKKeyType
	// Reveal returns each key's full value instead of the masked form.
	Reveal bool
}

func fromSDKSDKKey(stage string, s *sdkfm.SDKKeyDetails, reveal bool) SDKKey {
	value := s.SDKKey
	if !reveal {
		value = sdkfm.MaskSDKKey(s.SDKKey)
	}
	k := SDKKey{
		ReleaseStageKey:  stage,
		SDKKey:           value,
		Type:             s.Type,
		ModificationInfo: s.ModificationInfo,
		ResourceContext:  s.ResourceContext,
		Version:          s.Version,
	}
	if k.SDKKey == "" {
		k.SDKKey = redactedSDKKey
	}
	k.CreatedDisplay = "-"
	if mi := s.ModificationInfo; mi != nil {
		if !mi.CreatedTime.IsZero() {
			k.CreatedDisplay = displayTime(mi.CreatedTime)
		}
		k.CreatedByDisplay = mi.CreatedBy
	}
	return k
}

func createdTime(k SDKKey) time.Time {
	if k.ModificationInfo == nil {
		return time.Time{}
	}
	return k.ModificationInfo.CreatedTime
}

// ListSDKKeys returns a release stage's SDK keys, masked unless opts.Reveal,
// ordered by type (server, web, mobile) and then by creation time, keys
// without one last. The stage is a key or, when no stage has that key, a
// name.
func (h *Handler) ListSDKKeys(projectKey, releaseStageKeyOrName string, opts SDKKeyListOptions) ([]SDKKey, error) {
	var stage string
	list, err := withReleaseStageKey(h, projectKey, releaseStageKeyOrName, func(key string) (*sdkfm.SDKKeyList, error) {
		stage = key
		return h.sdk.ListSDKKeys(context.Background(), projectKey, key)
	})
	if err != nil {
		return nil, err
	}
	keys := []SDKKey{}
	for _, group := range [][]sdkfm.SDKKeyDetails{list.Server, list.Web, list.Mobile} {
		start := len(keys)
		for i := range group {
			if opts.Type != "" && group[i].Type != opts.Type {
				continue
			}
			keys = append(keys, fromSDKSDKKey(stage, &group[i], opts.Reveal))
		}
		sort.SliceStable(keys[start:], func(i, j int) bool {
			a, b := createdTime(keys[start+i]), createdTime(keys[start+j])
			if a.IsZero() || b.IsZero() {
				return !a.IsZero() && b.IsZero()
			}
			return a.Before(b)
		})
	}
	return keys, nil
}
