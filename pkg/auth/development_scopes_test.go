package auth

import (
	"slices"
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
