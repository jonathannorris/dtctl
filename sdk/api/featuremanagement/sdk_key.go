package featuremanagement

import (
	"context"
	"fmt"
	"strings"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// SDKKeyType is the kind of SDK an SDK key is for. Clients must tolerate
// values beyond the ones listed.
type SDKKeyType string

const (
	SDKKeyTypeWeb    SDKKeyType = "web"
	SDKKeyTypeServer SDKKeyType = "server"
	SDKKeyTypeMobile SDKKeyType = "mobile"
)

// SDKKeyDetails is one SDK key (schema SDKKeyDetails). SDKKey is the full
// credential, secret included: callers that display keys by default show
// MaskSDKKey instead.
type SDKKeyDetails struct {
	SDKKey           string            `json:"sdkKey" yaml:"sdkKey"`
	Type             SDKKeyType        `json:"type" yaml:"type"`
	ModificationInfo *ModificationInfo `json:"modificationInfo,omitempty" yaml:"modificationInfo,omitempty"`
	ResourceContext  *ResourceContext  `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
	Version          *int              `json:"version,omitempty" yaml:"version,omitempty"`
}

// SDKKeyList is a release stage's SDK keys grouped by type (schema
// SDKKeyList). It is not paged.
type SDKKeyList struct {
	Web             []SDKKeyDetails  `json:"web" yaml:"web"`
	Mobile          []SDKKeyDetails  `json:"mobile" yaml:"mobile"`
	Server          []SDKKeyDetails  `json:"server" yaml:"server"`
	ResourceContext *ResourceContext `json:"resourceContext,omitempty" yaml:"resourceContext,omitempty"`
}

// SDKKeyPublicID returns the public, log-safe identifier of an SDK key: its
// first two dot-separated parts, {token}.{platform}_{geo}_{publicRandom}. The
// third part is the secret. A value without that shape yields "", so a key in
// an unexpected format is never shown whole.
func SDKKeyPublicID(sdkKey string) string {
	parts := strings.SplitN(sdkKey, ".", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// maskedSecret replaces an SDK key's secret part in MaskSDKKey.
const maskedSecret = "********"

// MaskSDKKey returns an SDK key with its secret hidden: the public
// identifier followed by a fixed mask, {token}.{platform}_{geo}_{publicRandom}.********.
// The mask has a fixed length, so it says nothing about the secret. A value
// without the expected shape yields "", so it is never shown whole.
func MaskSDKKey(sdkKey string) string {
	id := SDKKeyPublicID(sdkKey)
	if id == "" {
		return ""
	}
	return id + "." + maskedSecret
}

// ListSDKKeys returns a release stage's SDK keys (listSdkKeys). The response
// carries every key's full value, secret included.
func (h *Handler) ListSDKKeys(ctx context.Context, projectKey, releaseStageKey string, addFields ...string) (*SDKKeyList, error) {
	return getOne[SDKKeyList](ctx, h.client,
		fmt.Sprintf("list SDK keys of release stage %q in project %q", releaseStageKey, projectKey),
		projectPath(projectKey, "release-stages", httpclient.PathSegment(releaseStageKey), "sdk-keys"), addFields)
}
