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

func TestContextAttributes_Handler(t *testing.T) {
	three := 3
	attrs := []sdkfm.ContextAttributeDetails{
		{Key: "plan-tier", Name: "Plan tier", Type: "STRING", ContextKey: "plan.tier", UsageCount: &three},
		{Key: "app-version", Name: "App version", Type: "SEM_VER", ContextKey: "app.version", BuiltIn: true},
	}
	base := sdkfm.BasePath + "/projects/checkout/context-attributes"
	var addFields []string
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		addFields = r.URL.Query()["add-fields"]
		_ = json.NewEncoder(w).Encode(sdkfm.ContextAttributeList{Items: attrs})
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, base+"/")
		if key, ok := strings.CutSuffix(rest, "/usage"); ok && key == "plan-tier" {
			_ = json.NewEncoder(w).Encode(sdkfm.ContextAttributeUsageList{Items: []sdkfm.ContextAttributeUsage{
				{FeatureKey: "zeta"}, {FeatureKey: "alpha"},
			}})
			return
		}
		for _, a := range attrs {
			if a.Key == rest {
				_ = json.NewEncoder(w).Encode(a)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(c)

	list, err := h.ListContextAttributes("checkout")
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Key != "app-version" || list[0].BuiltInDisplay != "yes" || list[0].UsagesDisplay != "-" {
		t.Errorf("first = %+v", list[0])
	}
	if list[1].UsagesDisplay != "3" {
		t.Errorf("usages display = %q", list[1].UsagesDisplay)
	}
	if strings.Join(addFields, ",") != "usagesCount" {
		t.Errorf("add-fields = %v", addFields)
	}

	a, err := h.GetContextAttribute("checkout", "Plan tier")
	if err != nil || a.Key != "plan-tier" {
		t.Fatalf("by name = %+v, %v", a, err)
	}
	usage, err := h.GetContextAttributeUsage("checkout", "plan-tier")
	if err != nil || len(usage) != 2 || usage[0].FeatureKey != "alpha" {
		t.Errorf("usage = %+v, %v", usage, err)
	}
}
