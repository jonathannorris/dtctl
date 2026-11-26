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

func fakeStages(t *testing.T, stages ...sdkfm.ReleaseStageDetails) *Handler {
	t.Helper()
	base := sdkfm.BasePath + "/projects/checkout/release-stages"
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.ReleaseStageList{Items: stages})
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, base+"/")
		if !keyPattern.MatchString(key) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
			return
		}
		for _, s := range stages {
			if s.Key == key {
				_ = json.NewEncoder(w).Encode(s)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Release stage not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(c)
}

func TestListReleaseStages_DisplayOrder(t *testing.T) {
	guarded := true
	h := fakeStages(t,
		sdkfm.ReleaseStageDetails{Key: "production", Name: "Production", Type: "PRODUCTION", Guarded: &guarded, DisplayOrder: 2},
		sdkfm.ReleaseStageDetails{Key: "development", Name: "Development", Type: "DEVELOPMENT", DisplayOrder: 0},
		sdkfm.ReleaseStageDetails{Key: "staging", Name: "Staging", Type: "STAGING", DisplayOrder: 1},
	)
	stages, err := h.ListReleaseStages("checkout")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range stages {
		keys = append(keys, s.Key)
	}
	if strings.Join(keys, ",") != "development,staging,production" {
		t.Errorf("order = %v", keys)
	}
	if stages[2].GuardedDisplay != "on" || stages[0].GuardedDisplay != "off" {
		t.Errorf("guarded display = %q, %q", stages[2].GuardedDisplay, stages[0].GuardedDisplay)
	}
}

func TestGetReleaseStage_ByName(t *testing.T) {
	h := fakeStages(t, sdkfm.ReleaseStageDetails{Key: "production", Name: "Prod EU", Type: "PRODUCTION"})
	rs, err := h.GetReleaseStage("checkout", "Prod EU")
	if err != nil {
		t.Fatal(err)
	}
	if rs.Key != "production" {
		t.Errorf("key = %q", rs.Key)
	}
	if _, err := h.GetReleaseStage("checkout", "nope"); err == nil || !strings.Contains(err.Error(), `release stage "nope" not found`) {
		t.Errorf("missing stage error = %v", err)
	}
}
