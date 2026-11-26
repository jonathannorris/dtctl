package featuremanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

func TestOverrides_Handler(t *testing.T) {
	base := sdkfm.BasePath + "/projects/checkout"
	on := "on"
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc(base+"/my-overrides", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.ProjectMyOverrides{Items: []sdkfm.FeatureMyOverrides{
			{FeatureKey: "zeta", Overrides: []sdkfm.ReleaseStageMyOverride{{ReleaseStageKey: "dev", VariantKey: "on"}}},
			{FeatureKey: "alpha", Overrides: []sdkfm.ReleaseStageMyOverride{
				{ReleaseStageKey: "dev", VariantKey: "off"}, {ReleaseStageKey: "staging", VariantKey: "on"}}},
		}})
	})
	mux.HandleFunc(base+"/overrides-profile", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"targetingKeys":["user-1"]}`))
	})
	mux.HandleFunc(base+"/features", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureList{Items: []sdkfm.FeatureDetails{{Key: "new-checkout", Name: "New checkout"}}})
	})
	mux.HandleFunc(base+"/features/new-checkout", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureDetails{Key: "new-checkout", Name: "New checkout"})
	})
	mux.HandleFunc(base+"/features/new-checkout/my-overrides", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.MyOverrides{Overrides: []sdkfm.MyOverride{
			{ReleaseStageKey: "dev"}, {ReleaseStageKey: "staging", VariantKey: &on, LastModifiedTime: &at},
		}})
	})
	mux.HandleFunc(base+"/features/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(c)

	items, err := h.ListOverrides("checkout")
	if err != nil {
		t.Fatal(err)
	}
	rows := ProjectOverrideRows(items)
	if len(rows) != 3 || rows[0].Feature != "alpha" || rows[2].Feature != "zeta" {
		t.Errorf("rows = %+v", rows)
	}

	fo, err := h.GetFeatureOverrides("checkout", "New checkout")
	if err != nil {
		t.Fatal(err)
	}
	if fo.FeatureKey != "new-checkout" {
		t.Errorf("feature key = %q", fo.FeatureKey)
	}
	frows := FeatureOverrideRows(fo)
	if frows[0].Variant != "-" || frows[0].LastModified != "-" || frows[1].Variant != "on" || frows[1].LastModified != "2026-11-03 10:00:00" {
		t.Errorf("feature rows = %+v", frows)
	}

	sum, err := h.GetOverridesSummary("checkout")
	if err != nil || len(sum.TargetingKeys) != 1 || len(sum.Items) != 2 {
		t.Errorf("summary = %+v, %v", sum, err)
	}
}
