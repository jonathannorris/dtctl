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
	"github.com/Dynatrace/dtctl/pkg/output"
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

// fmLeafScopes is the read scope each fm leaf requires, keyed by the leaf's
// command name on both verbs.
var fmLeafScopes = map[string]string{
	"projects":          "feature-management:projects:read",
	"project":           "feature-management:projects:read",
	"release-stages":    "feature-management:release-stages:read",
	"release-stage":     "feature-management:release-stages:read",
	"features":          "feature-management:features:read",
	"feature":           "feature-management:features:read",
	"feature-targeting": "feature-management:release-stage-targetings:read",
}

// With the opt-in, the catalog lists the group-qualified scope on each leaf
// and --check-scopes resolves the same requirement.
func TestFeatureManagementScopesResolve(t *testing.T) {
	applyDevelopmentRegistration(map[string]bool{config.DevelopmentAll: true})
	t.Cleanup(func() { applyDevelopmentRegistration(nil) })

	verbs := commands.BuildVerbs(rootCmd)
	for _, verb := range []string{"get", "describe"} {
		group, ok := verbs[verb].Subcommands["fm"]
		if !ok {
			t.Fatalf("%s fm is not in the catalog", verb)
		}
		for name, leaf := range group.Subcommands {
			want, ok := fmLeafScopes[name]
			if !ok {
				t.Errorf("%s fm %s has no entry in fmLeafScopes", verb, name)
				continue
			}
			if !slices.Equal(leaf.RequiredScopes, []string{want}) {
				t.Errorf("%s fm %s required scopes = %v, want [%s]", verb, name, leaf.RequiredScopes, want)
			}
		}
	}

	for _, path := range [][]string{{"get", "fm", "projects"}, {"get", "fm", "release-stages"}, {"describe", "fm", "release-stage"}} {
		c, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		want := fmLeafScopes[c.Name()]
		verb, resource := verbResource(c)
		got, req := scopesForInvocation(c, verb, resource)
		if req != scopeRequirementKnown || !slices.Equal(got, []string{want}) {
			t.Errorf("--check-scopes for %v = %v (%v), want [%s]", path, got, req, want)
		}
	}

	// The verdict names the command as typed, not a bare `get projects`.
	c, _, err := rootCmd.Find([]string{"get", "fm", "projects"})
	if err != nil {
		t.Fatal(err)
	}
	verb, resource := verbResource(c)
	if shown := shownResource(c, verb, resource); shown != "fm projects" {
		t.Errorf("shown resource = %q, want \"fm projects\"", shown)
	}
	msg := (&ScopeError{Verb: verb, Resource: shownResource(c, verb, resource), Missing: []string{fmLeafScopes["projects"]}}).Error()
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

func TestDescribeFMReleaseStageTableGolden(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	modified := time.Date(2026, 11, 3, 14, 30, 0, 0, time.UTC)
	on := true
	var buf bytes.Buffer
	printFMReleaseStageDescribe(&buf, "checkout", &featuremanagement.ReleaseStage{
		Key: "production", Name: "Production", Description: "Customer-facing traffic",
		Type: "PRODUCTION", Guarded: &on, GuardedDisplay: "on", DisplayOrder: 2, Version: 3,
		ModificationInfo: featuremanagement.ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-0000000000a1", CreatedTime: created,
			LastModifiedBy: "00000000-0000-4000-8000-0000000000a2", LastModifiedTime: modified,
		},
	})
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-release-stage/table", buf.String())
}

func fmFeatureForDescribe() *featuremanagement.Feature {
	zero, one := 0, 1
	return &featuremanagement.Feature{
		ProjectKey: "checkout", Key: "new-checkout", Name: "New checkout",
		Description: "One-page checkout",
		Variants: []featuremanagement.Variant{
			{Key: "baseline", Name: "Baseline", DisplayOrder: &zero},
			{Key: "variation-a", Name: "Variation A", DisplayOrder: &one},
		},
		Flags: []featuremanagement.FeatureFlag{
			{Key: "enabled", Name: "enabled", Type: "BOOLEAN", Variants: map[string]any{"baseline": false, "variation-a": true}},
			{Key: "button-color", Name: "Button color", Description: "Color of the pay button", Type: "STRING",
				Variants: map[string]any{"baseline": "blue", "variation-a": "green"},
				Schema:   map[string]any{"type": "ENUM", "values": []any{"blue", "green"}}},
		},
		Owner:       "00000000-0000-4000-8000-0000000000a1",
		Maintainers: []string{"00000000-0000-4000-8000-0000000000a2"},
		Status:      "ACTIVE", Type: "RELEASE", Version: 5,
		ModificationInfo: featuremanagement.ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-0000000000a1", CreatedTime: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
			LastModifiedBy: "00000000-0000-4000-8000-0000000000a2", LastModifiedTime: time.Date(2026, 11, 3, 14, 30, 0, 0, time.UTC),
		},
	}
}

func TestDescribeFMFeatureTableGolden(t *testing.T) {
	var buf bytes.Buffer
	printFMFeatureDescribe(&buf, fmFeatureForDescribe())
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-feature/table", buf.String())
}

func TestDescribeFMFeatureTableGolden_Empty(t *testing.T) {
	var buf bytes.Buffer
	printFMFeatureDescribe(&buf, &featuremanagement.Feature{
		ProjectKey: "checkout", Key: "bare", Name: "Bare", Status: "ARCHIVED", Type: "OPS", Version: 1,
	})
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-feature/table-empty", buf.String())
}

func fmTargetingForDescribe() *featuremanagement.FeatureTargeting {
	at := func(d int) time.Time { return time.Date(2026, 11, 24+d, 21, 0, 0, 0, time.UTC) }
	on := []featuremanagement.VariantDistribution{{VariantKey: "variation-a", Percentage: 100}}
	return &featuremanagement.FeatureTargeting{
		ProjectKey: "checkout", FeatureKey: "new-checkout", ReleaseStageKey: "production", Enabled: true, Version: 6,
		Targeting: featuremanagement.Targeting{Rules: []featuremanagement.TargetingRule{
			{ID: "6f1c2b9e-0000-4000-8000-000000000001", Serve: on, Conditions: []featuremanagement.TargetingCondition{
				{ValueType: "STRING", ContextKey: "targetingKey", Operator: "IS", Values: []any{"user-1", "user-2"}},
				{ValueType: "SEM_VER", ContextKey: "app.version", Operator: "GREATER_THAN_OR_EQUAL", Values: []any{"2.3.0"}},
			}},
			{ID: "a93d07e1-0000-4000-8000-000000000002", Name: "Everyone else", Serve: on, BucketingKey: "account.id",
				Rollout: &featuremanagement.Rollout{Steps: []featuremanagement.RolloutStep{
					{EffectiveTime: at(0), Percentage: 10},
					{EffectiveTime: at(1), Percentage: 50, Transition: "LINEAR"},
					{EffectiveTime: at(3), Percentage: 100},
				}}},
			{ID: "0c4e8a72-0000-4000-8000-000000000003", Serve: []featuremanagement.VariantDistribution{
				{VariantKey: "baseline", Percentage: 50}, {VariantKey: "variation-a", Percentage: 50}}},
		}},
		ModificationInfo: featuremanagement.ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-0000000000a1", CreatedTime: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
			LastModifiedBy: "00000000-0000-4000-8000-0000000000a2", LastModifiedTime: time.Date(2026, 11, 3, 14, 30, 0, 0, time.UTC),
		},
	}
}

func TestDescribeFMFeatureTargetingTableGolden(t *testing.T) {
	var buf bytes.Buffer
	printFMTargetingDescribe(&buf, fmTargetingForDescribe())
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-feature-targeting/table", buf.String())
}

func TestDescribeFMFeatureTargetingTableGolden_NoRules(t *testing.T) {
	var buf bytes.Buffer
	printFMTargetingDescribe(&buf, &featuremanagement.FeatureTargeting{
		FeatureKey: "new-checkout", ReleaseStageKey: "staging", Version: 1,
	})
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-feature-targeting/table-no-rules", buf.String())
}

func TestDescribeFMFeatureStagesGolden(t *testing.T) {
	var buf bytes.Buffer
	printFMFeatureStages(&buf, []featuremanagement.FeatureTargeting{
		{ReleaseStageKey: "development"},
		{ReleaseStageKey: "staging", Targeting: featuremanagement.Targeting{Rules: make([]featuremanagement.TargetingRule, 1)}},
		*fmTargetingForDescribe(),
	})
	cmdtestutil.AssertGoldenStripped(t, "describe-fm-feature/stages", buf.String())
}

// get fm feature-targeting prints rule rows in its tabular formats and the
// API's targeting otherwise.
func TestGetFMFeatureTargetingTableGolden(t *testing.T) {
	items := []featuremanagement.FeatureTargeting{
		{FeatureKey: "new-checkout", ReleaseStageKey: "development", Version: 1},
		*fmTargetingForDescribe(),
	}
	for _, format := range []string{"table", "wide", "csv"} {
		var buf bytes.Buffer
		if err := printFMTargetingTable(&buf, format, output.NewPrinterWithWriter(format, &buf), items, false); err != nil {
			t.Fatal(err)
		}
		cmdtestutil.AssertGoldenStripped(t, "get-fm-feature-targeting/"+format, buf.String())
	}
	// One stage (--stage): the rules follow the summary without a heading.
	for _, format := range []string{"table", "csv"} {
		var buf bytes.Buffer
		if err := printFMTargetingTable(&buf, format, output.NewPrinterWithWriter(format, &buf), items[1:], true); err != nil {
			t.Fatal(err)
		}
		cmdtestutil.AssertGoldenStripped(t, "get-fm-feature-targeting/"+format+"-one-stage", buf.String())
	}
}
