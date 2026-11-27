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
// login with the opt-in at readwrite-all requests every scope it lists, so a
// user allowed to run the command holds what it needs.
func TestDevelopmentCommandScopesAreRequested(t *testing.T) {
	enabled := map[string]bool{"feature-management": true}
	requested := LoginScopes(config.SafetyLevelReadWriteAll, enabled)
	for key, scopes := range developmentCommandScopes {
		_, resource, _ := strings.Cut(key, " ")
		if _, ok := developmentResources[resource]; !ok {
			t.Errorf("%q: %q is not a development resource", key, resource)
		}
		for _, s := range scopes {
			if !slices.Contains(requested, s) {
				t.Errorf("%q needs %q, which login at readwrite-all with the opt-in does not request", key, s)
			}
		}
	}
}
