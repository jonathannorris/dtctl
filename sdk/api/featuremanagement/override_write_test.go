package featuremanagement

import (
	"context"
	"testing"
)

func TestOverrideWrites_MethodsPathsAndBodies(t *testing.T) {
	h, calls := recordWrites(t, `{"overrides":[]}`)
	ctx := context.Background()
	on := "on"
	if _, err := h.UpdateMyOverrides(ctx, "checkout", "a:b", MyOverridesUpdate{Overrides: []MyOverrideUpdate{{ReleaseStageKey: "staging", VariantKey: &on}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.UpdateMyOverrides(ctx, "checkout", "a:b", MyOverridesUpdate{}); err != nil {
		t.Fatal(err)
	}
	if err := h.UpdateOverridesProfile(ctx, "checkout", OverridesProfile{}); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteProjectMyOverrides(ctx, "checkout"); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteOverridesProfile(ctx, "checkout"); err != nil {
		t.Fatal(err)
	}
	want := []writeCall{
		{"PUT", "/projects/checkout/features/a%3Ab/my-overrides", "", `{"overrides":[{"releaseStageKey":"staging","variantKey":"on"}]}`},
		{"PUT", "/projects/checkout/features/a%3Ab/my-overrides", "", `{"overrides":[]}`},
		{"PUT", "/projects/checkout/overrides-profile", "", `{"targetingKeys":[]}`},
		{"DELETE", "/projects/checkout/my-overrides", "", ""},
		{"DELETE", "/projects/checkout/overrides-profile", "", ""},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
}
