package featuremanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

func intPtr(i int) *int { return &i }

func fakeFeatures(t *testing.T, features ...sdkfm.FeatureDetails) *Handler {
	t.Helper()
	base := sdkfm.BasePath + "/projects/checkout/features"
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureList{Items: features})
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, base+"/")
		if !keyPattern.MatchString(key) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
			return
		}
		for _, f := range features {
			if f.Key == key {
				_ = json.NewEncoder(w).Encode(f)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Feature not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(c)
}

func TestListFeatures_SortedWithDisplay(t *testing.T) {
	h := fakeFeatures(t,
		sdkfm.FeatureDetails{Key: "zeta", Name: "Zeta", Status: "ACTIVE", Type: "RELEASE",
			Variants: []sdkfm.Variant{{Key: "on", DisplayOrder: intPtr(1)}, {Key: "off", DisplayOrder: intPtr(0)}},
			Flags:    []sdkfm.FeatureFlag{{Key: "enabled"}, {Key: "color"}}},
		sdkfm.FeatureDetails{Key: "alpha", Name: "Alpha", Status: "ARCHIVED", Type: "OPS"},
	)
	features, err := h.ListFeatures("checkout")
	if err != nil {
		t.Fatal(err)
	}
	if features[0].Key != "alpha" || features[1].Key != "zeta" {
		t.Fatalf("order = %s, %s", features[0].Key, features[1].Key)
	}
	if got := features[1].VariantsDisplay; got != "off,on" {
		t.Errorf("variants display = %q, want display order", got)
	}
	if got := features[1].FlagsDisplay; got != "2" {
		t.Errorf("flags display = %q", got)
	}
	if features[0].Variants == nil || features[0].Flags == nil || features[0].Maintainers == nil {
		t.Errorf("absent lists should print as [], got %+v", features[0])
	}
}

func TestGetFeature_ByName(t *testing.T) {
	h := fakeFeatures(t, sdkfm.FeatureDetails{Key: "new-checkout", Name: "New checkout"})
	f, err := h.GetFeature("checkout", "New checkout")
	if err != nil {
		t.Fatal(err)
	}
	if f.Key != "new-checkout" {
		t.Errorf("key = %q", f.Key)
	}
}
