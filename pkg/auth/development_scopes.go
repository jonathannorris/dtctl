package auth

import (
	"sort"

	"github.com/Dynatrace/dtctl/pkg/config"
	"github.com/Dynatrace/dtctl/pkg/stability"
	"github.com/Dynatrace/dtctl/sdk/session"
)

// developmentResources maps each ResourceScopes entry that belongs to a
// development-tier feature to that feature's opt-in key. Login requests these
// resources' scopes only when the feature is enabled, so a released build's
// login scope list does not change because an unfinished command exists in
// the tree. The safety-level composition (safetyLevelScopes) never reads them.
var developmentResources = map[string]string{
	"fm/project":           "feature-management",
	"fm/release-stage":     "feature-management",
	"fm/feature":           "feature-management",
	"fm/feature-targeting": "feature-management",
}

// DevelopmentResourceFeatures returns a copy of the resource-to-feature map,
// for the cmd test that checks every key names a registered feature.
func DevelopmentResourceFeatures() map[string]string {
	out := make(map[string]string, len(developmentResources))
	for r, f := range developmentResources {
		out[r] = f
	}
	return out
}

// LoginScopes returns the scopes interactive login requests: the safety
// level's set, plus the scopes of development-tier resources whose feature is
// in enabled. With nothing enabled it equals GetScopesForSafetyLevel.
//
// Only read access is added for now: every safety level grants reads, and the
// gated resources have no write paths yet. When one gains them, its write
// scopes join here under the same per-level rules as addMineWrites and
// addAllExtras.
func LoginScopes(level config.SafetyLevel, enabled map[string]bool) []string {
	s := newScopeSet()
	s.add(safetyLevelScopes(level)...)
	resources := make([]string, 0, len(developmentResources))
	for r := range developmentResources {
		resources = append(resources, r)
	}
	sort.Strings(resources)
	for _, resource := range resources {
		if stability.Enabled(developmentResources[resource], enabled) {
			s.addResource(resource, AccessRead)
		}
	}
	return s.list
}

// OAuthConfigFromEnvironmentURLForLogin is OAuthConfigFromEnvironmentURLWithSafety
// with LoginScopes in place of the safety level's set.
func OAuthConfigFromEnvironmentURLForLogin(environmentURL string, safetyLevel config.SafetyLevel, enabled map[string]bool) *OAuthConfig {
	return session.OAuthConfigFromEnvironmentURL(environmentURL, safetyLevel, LoginScopes(safetyLevel, enabled))
}
