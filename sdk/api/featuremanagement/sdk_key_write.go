package featuremanagement

import (
	"context"
	"fmt"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// SDKKeyCreate is the body of an SDK key create (schema SdkKeyCreate).
type SDKKeyCreate struct {
	Type SDKKeyType `json:"type" yaml:"type"`
}

func sdkKeysPath(projectKey, releaseStageKey string, segments ...string) string {
	return projectPath(projectKey, append([]string{"release-stages", httpclient.PathSegment(releaseStageKey), "sdk-keys"}, segments...)...)
}

// CreateSDKKey creates an SDK key in a release stage (createSdkKey). The
// response carries the full key, secret included.
func (h *Handler) CreateSDKKey(ctx context.Context, projectKey, releaseStageKey string, body SDKKeyCreate) (*SDKKeyDetails, error) {
	return postOne[SDKKeyDetails](ctx, h.client,
		fmt.Sprintf("create %s SDK key in release stage %q of project %q", body.Type, releaseStageKey, projectKey),
		sdkKeysPath(projectKey, releaseStageKey), body)
}

// DeleteSDKKey revokes an SDK key (deleteSdkKey). The path takes the full
// key; errors name only its public identifier, so the secret never reaches
// an error message.
func (h *Handler) DeleteSDKKey(ctx context.Context, projectKey, releaseStageKey, sdkKey string) error {
	return deleteOne(ctx, h.client,
		fmt.Sprintf("delete SDK key %s of release stage %q in project %q", MaskSDKKey(sdkKey), releaseStageKey, projectKey),
		sdkKeysPath(projectKey, releaseStageKey, httpclient.PathSegment(sdkKey)))
}
