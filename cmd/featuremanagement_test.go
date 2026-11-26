package cmd

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	cmdtestutil "github.com/Dynatrace/dtctl/cmd/testutil"
	"github.com/Dynatrace/dtctl/pkg/auth"
	"github.com/Dynatrace/dtctl/pkg/commands"
	"github.com/Dynatrace/dtctl/pkg/config"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/stability"
)

// Feature Management is one development feature: a single opt-in must grant
// the fm group on every verb, or the stability floor keeps blocking the rest.
func TestFeatureManagementIsOneDevelopmentFeature(t *testing.T) {
	want := []string{"get fm", "describe fm"}
	reg := stability.DefaultRegistry()
	paths := reg.Paths(featureManagementDevelopmentFeature)
	granted := reg.EnabledPaths(map[string]bool{featureManagementDevelopmentFeature: true})
	for _, p := range want {
		if !slices.Contains(paths, p) {
			t.Errorf("feature %q does not gate %q (gates %v)", featureManagementDevelopmentFeature, p, paths)
		}
		if !slices.Contains(granted, p) {
			t.Errorf("enabling %q does not grant %q (grants %v)", featureManagementDevelopmentFeature, p, granted)
		}
	}
}

// Without the opt-in the fm group is not in the tree at all.
func TestFeatureManagementNotRegisteredByDefault(t *testing.T) {
	applyDevelopmentRegistration(nil)
	for _, verb := range []string{"get", "describe"} {
		if c, _, err := rootCmd.Find([]string{verb, "fm"}); err == nil && c.Name() == "fm" {
			t.Errorf("%s fm is registered without the opt-in", verb)
		}
	}
}

// Every development-gated scope entry names a feature the registry knows, so
// a renamed feature key cannot leave login asking for scopes nobody can opt
// into (or never asking).
func TestDevelopmentResourceFeaturesAreRegistered(t *testing.T) {
	features := stability.DefaultRegistry().Features()
	for resource, feature := range auth.DevelopmentResourceFeatures() {
		if !slices.Contains(features, feature) {
			t.Errorf("auth gates %q on feature %q, which no command declares (known: %v)", resource, feature, features)
		}
	}
}

// With the opt-in, the catalog lists the group-qualified scope on the leaf
// and --check-scopes resolves the same requirement.
func TestFeatureManagementScopesResolve(t *testing.T) {
	applyDevelopmentRegistration(map[string]bool{config.DevelopmentAll: true})
	t.Cleanup(func() { applyDevelopmentRegistration(nil) })

	const scope = "feature-management:projects:read"
	verbs := commands.BuildVerbs(rootCmd)
	for _, verb := range []string{"get", "describe"} {
		group, ok := verbs[verb].Subcommands["fm"]
		if !ok {
			t.Fatalf("%s fm is not in the catalog", verb)
		}
		for name, leaf := range group.Subcommands {
			if !slices.Equal(leaf.RequiredScopes, []string{scope}) {
				t.Errorf("%s fm %s required scopes = %v, want [%s]", verb, name, leaf.RequiredScopes, scope)
			}
		}
	}

	c, _, err := rootCmd.Find([]string{"get", "fm", "projects"})
	if err != nil {
		t.Fatal(err)
	}
	verb, resource := verbResource(c)
	got, req := scopesForInvocation(c, verb, resource)
	if req != scopeRequirementKnown || !slices.Equal(got, []string{scope}) {
		t.Errorf("--check-scopes for get fm projects = %v (%v), want [%s]", got, req, scope)
	}

	// The verdict names the command as typed, not a bare `get projects`.
	if shown := shownResource(c, verb, resource); shown != "fm projects" {
		t.Errorf("shown resource = %q, want \"fm projects\"", shown)
	}
	msg := (&ScopeError{Verb: verb, Resource: shownResource(c, verb, resource), Missing: []string{scope}}).Error()
	if !strings.Contains(msg, `"get fm projects"`) {
		t.Errorf("scope error = %q, want it to name \"get fm projects\"", msg)
	}
}

// The older groups key their scopes by leaf name, so their verdicts keep
// naming the bare leaf exactly as before.
func TestShownResourceLeavesOlderGroupsAlone(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"get", "aws", "connections"})
	if err != nil || c.Name() == "aws" {
		t.Skipf("get aws connections not found: %v", err)
	}
	verb, resource := verbResource(c)
	if shown := shownResource(c, verb, resource); shown != resource {
		t.Errorf("shown resource = %q, want %q", shown, resource)
	}
}

func fmProjectForDescribe() *featuremanagement.Project {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	modified := time.Date(2026, 11, 3, 14, 30, 0, 0, time.UTC)
	on := true
	return &featuremanagement.Project{
		Key:                   "checkout",
		Name:                  "Checkout",
		Description:           "Flags for the checkout flow",
		Owner:                 "00000000-0000-4000-8000-0000000000a1",
		Maintainers:           []string{"00000000-0000-4000-8000-0000000000a2", "00000000-0000-4000-8000-0000000000a3"},
		Settings:              featuremanagement.ProjectSettings{ChangeRequests: &featuremanagement.ChangeRequestSettings{Enabled: &on}},
		ChangeRequestsDisplay: "on",
		ModificationInfo: featuremanagement.ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-0000000000a1", CreatedTime: created,
			LastModifiedBy: "00000000-0000-4000-8000-0000000000a2", LastModifiedTime: modified,
		},
		ResourceContext: &featuremanagement.ResourceContext{Operations: []string{"write", "delete"}},
		Version:         4,
	}
}

func TestDescribeFMProjectTableGolden(t *testing.T) {
	var buf bytes.Buffer
	printFMProjectDescribe(&buf, fmProjectForDescribe())
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-project/table", buf.String())
}

func TestDescribeFMProjectTableGolden_Minimal(t *testing.T) {
	var buf bytes.Buffer
	printFMProjectDescribe(&buf, &featuremanagement.Project{
		Key: "payments", Name: "Payments", ChangeRequestsDisplay: "off", Version: 1,
	})
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-project/table-minimal", buf.String())
}
