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
	"fm/context-attribute": "feature-management",
	"fm/sdk-key":           "feature-management",
	"fm/override":          "feature-management",
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

// developmentSharedWrites is the access each development-tier resource adds
// from readwrite-all upward. Feature Management configuration is shared, so
// its writes are refused at readwrite-mine (design decision 12) and their
// scopes are not requested there.
var developmentSharedWrites = map[string][]Access{
	"fm/feature":           {AccessWrite, AccessDelete},
	"fm/feature-targeting": {AccessWrite, AccessDelete},
}

// developmentSharedWriteExtras are scopes a development-tier resource's
// writes need beyond its ResourceScopes entry, requested with
// developmentSharedWrites. A feature's descriptive update has a scope of its
// own, so that tidying descriptions can be granted without behavior changes.
var developmentSharedWriteExtras = map[string][]string{
	"fm/feature": {"feature-management:features.metadata:write"},
}

// developmentCommandScopes is CommandScopes for development-tier commands:
// what a command needs when its resource's scopes at its access level do not
// say it. It is kept apart from CommandScopes, whose entries every safety
// level's login must cover, because login requests these only with the
// feature's opt-in (LoginScopes).
var developmentCommandScopes = map[string][]string{
	// Targeting writes read the stage first, and a rule write also reads the
	// feature (for its variants) and the context attributes (for each
	// condition's value type). enable and disable fm feature write the
	// targeting, not the feature.
	"create fm/feature-targeting": fmRuleWriteScopes,
	"update fm/feature-targeting": fmRuleWriteScopes,
	"delete fm/feature-targeting": fmTargetingWriteScopes,
	"enable fm/feature":           fmTargetingWriteScopes,
	"disable fm/feature":          fmTargetingWriteScopes,

	// Feature updates and deletes read the feature first. A descriptive
	// update writes with features.metadata:write instead of features:write;
	// the catalog lists the full update's scope, the common case.
	"update fm/feature": {"feature-management:features:read", "feature-management:features:write"},
	"delete fm/feature": {"feature-management:features:read", "feature-management:features:delete"},
}

var (
	fmTargetingWriteScopes = []string{
		"feature-management:release-stage-targetings:read",
		"feature-management:release-stage-targetings:write",
	}
	fmRuleWriteScopes = append(append([]string{}, fmTargetingWriteScopes...),
		"feature-management:features:read",
		"feature-management:context-attributes:read",
	)
)

// LoginScopes returns the scopes interactive login requests: the safety
// level's set, plus the scopes of development-tier resources whose feature is
// in enabled. With nothing enabled it equals GetScopesForSafetyLevel.
//
// Every level gets the gated resources' read scopes. Their write scopes
// follow the same per-level rules as addAllExtras: developmentSharedWrites
// joins from readwrite-all upward.
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
			if grantsSharedWrites(level) {
				s.addResource(resource, developmentSharedWrites[resource]...)
				s.add(developmentSharedWriteExtras[resource]...)
			}
		}
	}
	return s.list
}

// OAuthConfigFromEnvironmentURLForLogin is OAuthConfigFromEnvironmentURLWithSafety
// with LoginScopes in place of the safety level's set.
func OAuthConfigFromEnvironmentURLForLogin(environmentURL string, safetyLevel config.SafetyLevel, enabled map[string]bool) *OAuthConfig {
	return session.OAuthConfigFromEnvironmentURL(environmentURL, safetyLevel, LoginScopes(safetyLevel, enabled))
}

// grantsSharedWrites reports whether a safety level allows modifying shared
// resources: readwrite-all and dangerously-unrestricted.
func grantsSharedWrites(level config.SafetyLevel) bool {
	if level == "" {
		level = config.DefaultSafetyLevel
	}
	return level == config.SafetyLevelReadWriteAll || level == config.SafetyLevelDangerouslyUnrestricted
}
