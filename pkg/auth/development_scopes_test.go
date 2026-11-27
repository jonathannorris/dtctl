package auth

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/pkg/config"
)

// Without an opt-in, login asks for exactly what it asked for before any
// development resource existed: a released build's scope list is unchanged.
func TestLoginScopes_UnchangedWithoutOptIn(t *testing.T) {
	for _, level := range config.ValidSafetyLevels() {
		for _, enabled := range []map[string]bool{nil, {}, {"some-other-feature": true}} {
			got := LoginScopes(level, enabled)
			want := GetScopesForSafetyLevel(level)
			if !slices.Equal(got, want) {
				t.Errorf("LoginScopes(%s, %v) = %v, want the safety level's set %v", level, enabled, got, want)
			}
		}
	}
}

func TestLoginScopes_AddsReadScopesOfEnabledFeature(t *testing.T) {
	const scope = "feature-management:projects:read"
	for _, enabled := range []map[string]bool{
		{"feature-management": true},
		{config.DevelopmentAll: true},
	} {
		for _, level := range config.ValidSafetyLevels() {
			got := LoginScopes(level, enabled)
			if !slices.Contains(got, scope) {
				t.Errorf("LoginScopes(%s, %v) lacks %q", level, enabled, scope)
			}
			// Additive only: every scope of the level's set is still there.
			for _, s := range GetScopesForSafetyLevel(level) {
				if !slices.Contains(got, s) {
					t.Errorf("LoginScopes(%s, %v) dropped %q", level, enabled, s)
				}
			}
		}
	}
}

// Every gated resource names an entry in the canonical table, so the gate
// cannot silently guard nothing.
func TestDevelopmentResourcesHaveScopeEntries(t *testing.T) {
	for resource := range developmentResources {
		if _, ok := ResourceScopes[resource]; !ok {
			t.Errorf("developmentResources[%q] has no ResourceScopes entry", resource)
		}
	}
}

// Feature Management writes change shared configuration, so their scopes are
// requested from readwrite-all upward and never below (design decision 12).
func TestLoginScopes_SharedWritesFromReadWriteAllUp(t *testing.T) {
	const scope = "feature-management:release-stage-targetings:write"
	enabled := map[string]bool{"feature-management": true}
	want := map[config.SafetyLevel]bool{
		config.SafetyLevelReadOnly:                false,
		config.SafetyLevelReadWriteMine:           false,
		config.SafetyLevelReadWriteAll:            true,
		config.SafetyLevelDangerouslyUnrestricted: true,
	}
	for level, wantScope := range want {
		if got := slices.Contains(LoginScopes(level, enabled), scope); got != wantScope {
			t.Errorf("LoginScopes(%s) has %q = %v, want %v", level, scope, got, wantScope)
		}
	}
}

// Every development command-scope entry names a development resource, and
// login with the opt-in at the lowest level that runs the command requests
// every scope it lists, so a user allowed to run the command holds what it
// needs: readwrite-all, or dangerously-unrestricted for the deletes gated
// like a bucket delete.
func TestDevelopmentCommandScopesAreRequested(t *testing.T) {
	enabled := map[string]bool{"feature-management": true}
	for key, scopes := range developmentCommandScopes {
		verb, resource, _ := strings.Cut(key, " ")
		if _, ok := developmentResources[resource]; !ok {
			t.Errorf("%q: %q is not a development resource", key, resource)
		}
		level := config.SafetyLevelReadWriteAll
		if verb == "delete" && slices.Contains(developmentUnrestrictedAccess[resource], AccessDelete) {
			level = config.SafetyLevelDangerouslyUnrestricted
		}
		requested := LoginScopes(level, enabled)
		for _, s := range scopes {
			if !slices.Contains(requested, s) {
				t.Errorf("%q needs %q, which login at %s with the opt-in does not request", key, s, level)
			}
		}
	}
}

// Project, release stage and SDK key deletes take what depends on them
// along (for an SDK key, the applications using it), so
// dtctl gates them like a bucket delete and requests their scopes only at
// dangerously-unrestricted.
func TestLoginScopes_ProjectDeleteOnlyWhenUnrestricted(t *testing.T) {
	enabled := map[string]bool{"feature-management": true}
	for level, want := range map[config.SafetyLevel]bool{
		config.SafetyLevelReadWriteMine:           false,
		config.SafetyLevelReadWriteAll:            false,
		config.SafetyLevelDangerouslyUnrestricted: true,
	} {
		for _, scope := range []string{"feature-management:projects:delete", "feature-management:release-stages:delete", "feature-management:sdk-keys:delete"} {
			if got := slices.Contains(LoginScopes(level, enabled), scope); got != want {
				t.Errorf("LoginScopes(%s) has %s = %v, want %v", level, scope, got, want)
			}
		}
	}
	if !slices.Contains(LoginScopes(config.SafetyLevelReadWriteAll, enabled), "feature-management:projects:write") {
		t.Error("LoginScopes(readwrite-all) lacks projects:write")
	}
}
