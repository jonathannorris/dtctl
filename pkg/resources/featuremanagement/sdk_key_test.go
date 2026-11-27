package featuremanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
	"gopkg.in/yaml.v3"
)

const testSDKKeySecret = "0123456789abcdef0123456789abcdef_0123456789"

func TestListSDKKeys_MaskedRevealedAndFiltered(t *testing.T) {
	older := &sdkfm.ModificationInfo{CreatedBy: "u1", CreatedTime: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}
	newer := &sdkfm.ModificationInfo{CreatedBy: "u2", CreatedTime: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)}
	list := sdkfm.SDKKeyList{
		Web: []sdkfm.SDKKeyDetails{{SDKKey: "dt01.web_us_Webkey0001." + testSDKKeySecret, Type: "web"}},
		Server: []sdkfm.SDKKeyDetails{
			{SDKKey: "dt01.server_us_Newer00001." + testSDKKeySecret, Type: "server", ModificationInfo: newer},
			{SDKKey: "dt01.server_us_Older00001." + testSDKKeySecret, Type: "server", ModificationInfo: older},
			{SDKKey: "weird-" + testSDKKeySecret, Type: "server"},
		},
	}
	stages := []sdkfm.ReleaseStageDetails{{Key: "production", Name: "Production"}}
	base := sdkfm.BasePath + "/projects/checkout/release-stages"
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.ReleaseStageList{Items: stages})
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, base+"/") {
		case "production/sdk-keys":
			_ = json.NewEncoder(w).Encode(list)
		case "production":
			_ = json.NewEncoder(w).Encode(stages[0])
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(c)
	keys, err := h.ListSDKKeys("checkout", "Production", SDKKeyListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	for _, k := range keys {
		values = append(values, k.SDKKey)
		if k.ReleaseStageKey != "production" {
			t.Errorf("stage = %q", k.ReleaseStageKey)
		}
	}
	want := "dt01.server_us_Older00001.********,dt01.server_us_Newer00001.********,(unrecognized key format),dt01.web_us_Webkey0001.********"
	if got := strings.Join(values, ","); got != want {
		t.Errorf("keys = %s\nwant   %s", got, want)
	}

	// Without --reveal no format may carry the secret.
	js, _ := json.Marshal(keys)
	ys, _ := yaml.Marshal(keys)
	for name, out := range map[string]string{"json": string(js), "yaml": string(ys)} {
		if strings.Contains(out, "0123456789abcdef") {
			t.Errorf("%s output contains the key secret:\n%s", name, out)
		}
	}

	// --reveal returns full values; --type narrows the list.
	keys, err = h.ListSDKKeys("checkout", "production", SDKKeyListOptions{Type: "web", Reveal: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].SDKKey != "dt01.web_us_Webkey0001."+testSDKKeySecret || keys[0].Type != "web" {
		t.Errorf("revealed web keys = %+v", keys)
	}
}
