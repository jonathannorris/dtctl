package featuremanagement

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

const featureWithUnknownJSON = `{"projectKey":"checkout","key":"new-checkout","name":"New checkout","description":"d",
 "variants":[{"key":"off","name":"Off","displayOrder":0,"resourceContext":{"operations":["write"]}},{"key":"on","name":"On","displayOrder":1}],
 "flags":[{"key":"layout","name":"Layout","description":"","type":"STRING","variants":{"off":"classic","on":"one-page"},
           "schema":{"type":"ENUM","values":["classic","one-page"]},"tags":["ui"]}],
 "owner":"team","maintainers":["a"],"status":"ACTIVE","type":"RELEASE","version":3,
 "links":[{"url":"https://example.invalid/ticket"}],
 "modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z","lastModifiedBy":"u","lastModifiedTime":"2026-11-03T10:00:00Z"}}`

func TestNewFeatureUpdate_CarriesUnknownMembers(t *testing.T) {
	var d FeatureDetails
	if err := json.Unmarshal([]byte(featureWithUnknownJSON), &d); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(NewFeatureUpdate(&d))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(out, &body)
	if body["links"] == nil {
		t.Errorf("links dropped: %s", out)
	}
	for _, readOnly := range []string{"projectKey", "key", "version", "modificationInfo"} {
		if _, has := body[readOnly]; has {
			t.Errorf("read-only %q sent: %s", readOnly, out)
		}
	}
	flag := body["flags"].([]any)[0].(map[string]any)
	if flag["tags"] == nil || flag["schema"] == nil {
		t.Errorf("flag members dropped: %v", flag)
	}
	if _, has := body["variants"].([]any)[0].(map[string]any)["resourceContext"]; has {
		t.Errorf("variant resourceContext sent: %s", out)
	}
}

func TestNewFeatureMetadataUpdate_DescriptiveFieldsOnly(t *testing.T) {
	var d FeatureDetails
	if err := json.Unmarshal([]byte(featureWithUnknownJSON), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(NewFeatureMetadataUpdate(&d))
	s := string(out)
	for _, absent := range []string{`"status"`, `"schema"`, `"one-page"`, `"version"`} {
		if strings.Contains(s, absent) {
			t.Errorf("metadata update carries %s: %s", absent, s)
		}
	}
	if !strings.Contains(s, `"links"`) || !strings.Contains(s, `{"key":"layout","name":"Layout","description":""}`) {
		t.Errorf("metadata update = %s", s)
	}
}

func TestFeatureWrites_MethodsPathsAndVersions(t *testing.T) {
	type call struct{ method, path, version, body string }
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, call{r.Method, r.URL.EscapedPath(), r.URL.Query().Get("optimistic-locking-version"), string(b)})
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"version":"4"}`))
		default:
			_, _ = w.Write([]byte(featureWithUnknownJSON))
		}
	}))
	defer srv.Close()
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	h, ctx := NewHandler(c), context.Background()

	if _, err := h.CreateFeature(ctx, "checkout", FeatureCreate{Key: "new-checkout", Name: "N", Variants: []Variant{{Key: "off", Name: "off"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.UpdateFeature(ctx, "checkout", "new-checkout", FeatureUpdate{}, "3"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.UpdateFeatureMetadata(ctx, "checkout", "new-checkout", FeatureMetadataUpdate{}, "3"); err != nil {
		t.Fatal(err)
	}
	v, err := h.PatchFeature(ctx, "checkout", "new-checkout", FeaturePatch{Status: FeatureStatusArchived}, "3")
	if err != nil || v.Version.String() != "4" {
		t.Fatalf("patch = %v, %v", v, err)
	}
	if err := h.DeleteFeature(ctx, "checkout", "new-checkout"); err != nil {
		t.Fatal(err)
	}

	f := BasePath + "/projects/checkout/features"
	want := []call{
		{"POST", f, "", ""},
		{"PUT", f + "/new-checkout", "3", ""},
		{"PUT", f + "/new-checkout/metadata", "3", ""},
		{"PATCH", f + "/new-checkout", "3", `{"status":"ARCHIVED"}`},
		{"DELETE", f + "/new-checkout", "", ""},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v", calls)
	}
	for i, w := range want {
		got := calls[i]
		if got.method != w.method || got.path != w.path || got.version != w.version || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
	if !strings.Contains(calls[0].body, `"variants":[{"key":"off","name":"off"}]`) {
		t.Errorf("create body = %s", calls[0].body)
	}
}
