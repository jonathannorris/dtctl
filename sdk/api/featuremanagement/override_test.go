package featuremanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func TestOverrides(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case BasePath + "/projects/checkout/my-overrides":
			_, _ = w.Write([]byte(`{"items":[{"featureKey":"new-checkout","featureName":"New checkout","overrides":[{"releaseStageKey":"staging","releaseStageName":"Staging","variantKey":"on","variantName":"On"}]}]}`))
		case BasePath + "/projects/checkout/features/a%3Ab/my-overrides":
			_, _ = w.Write([]byte(`{"overrides":[{"releaseStageKey":"development","variantKey":null,"lastModifiedTime":null},{"releaseStageKey":"staging","variantKey":"on","lastModifiedTime":"2026-11-03T10:00:00Z"}]}`))
		case BasePath + "/projects/checkout/overrides-profile":
			_, _ = w.Write([]byte(`{"targetingKeys":["user-1","Device-2"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(c)
	ctx := context.Background()

	all, err := h.GetProjectMyOverrides(ctx, "checkout")
	if err != nil || len(all.Items) != 1 || all.Items[0].Overrides[0].VariantKey != "on" {
		t.Fatalf("project overrides = %+v, %v", all, err)
	}
	one, err := h.GetMyOverrides(ctx, "checkout", "a:b")
	if err != nil {
		t.Fatal(err)
	}
	if o := one.Overrides; len(o) != 2 || o[0].VariantKey != nil || o[0].LastModifiedTime != nil || *o[1].VariantKey != "on" || o[1].LastModifiedTime == nil {
		t.Errorf("feature overrides = %+v", o)
	}
	p, err := h.GetOverridesProfile(ctx, "checkout")
	if err != nil || len(p.TargetingKeys) != 2 || p.TargetingKeys[1] != "Device-2" {
		t.Errorf("profile = %+v, %v", p, err)
	}
}
