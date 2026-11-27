package featuremanagement

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

func TestFeatureCreateSpec_Body(t *testing.T) {
	body, err := FeatureCreateSpec{Key: "new-checkout", Name: "New checkout"}.Body()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(body)
	want := `{"key":"new-checkout","name":"New checkout","flag":{"key":"new-checkout","type":"BOOLEAN"},` +
		`"variants":[{"key":"disabled","name":"disabled","displayOrder":0},{"key":"enabled","name":"enabled","displayOrder":1}]}`
	if string(got) != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}

	body, err = FeatureCreateSpec{Key: "f", Name: "F", Variants: []string{"va", "vb"}, FlagKey: "layout", FlagType: "STRING",
		FlagValues: []string{"va=classic", "vb=one-page"}}.Body()
	if err != nil || body.Flag.Variants["vb"] != "one-page" || body.Flag.Key != "layout" {
		t.Errorf("body = %+v, %v", body.Flag, err)
	}
	if _, err := (FeatureCreateSpec{Key: "f", Name: "F", FlagValues: []string{"enabled=true"}}).Body(); err == nil || !strings.Contains(err.Error(), "missing: disabled") {
		t.Errorf("partial values err = %v", err)
	}
	if _, err := (FeatureCreateSpec{Key: "f", Name: "F", Variants: []string{"on", "a"}}).Body(); err == nil || !strings.Contains(err.Error(), `invalid variant key "a"`) {
		t.Errorf("short variant key err = %v", err)
	}
}

func TestCheckVariantKey(t *testing.T) {
	for _, k := range []string{"on", "a1", "variation-a", "v1.2_beta", "0x"} {
		if err := checkVariantKey("--add-variant", k); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
	for _, k := range []string{"a", "A1", "-on", "on-", "o n", strings.Repeat("a", 257)} {
		if err := checkVariantKey("--add-variant", k); err == nil {
			t.Errorf("%q: expected an error", k)
		}
	}
}

func TestParseFlagValueAndDefaults(t *testing.T) {
	for _, tt := range []struct {
		t    FlagType
		raw  string
		want string
	}{
		{"BOOLEAN", "true", "true"}, {"NUMBER", "2.5", "2.5"}, {"STRING", "a b", `"a b"`}, {"JSON", `{"a":1}`, `{"a":1}`},
	} {
		v, err := ParseFlagValue(tt.t, tt.raw)
		got, _ := json.Marshal(v)
		if err != nil || string(got) != tt.want {
			t.Errorf("ParseFlagValue(%s, %q) = %s, %v", tt.t, tt.raw, got, err)
		}
	}
	if _, err := ParseFlagValue("NUMBER", "x"); err == nil {
		t.Error("NUMBER x: expected an error")
	}
}

func TestFeatureFile_Decode(t *testing.T) {
	var body FeatureCreate
	yaml := "kind: fm/feature\nprojectKey: checkout\nkey: f\nname: F\nstatus: ACTIVE\nversion: 3\nlinks: [{url: x}]\nvariants: [{key: a, name: A}]\n"
	if err := FeatureFile("checkout").Decode([]byte(yaml), &body); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(body)
	if s := string(got); strings.Contains(s, "status") || strings.Contains(s, "version") || strings.Contains(s, "kind") || !strings.Contains(s, `"links"`) {
		t.Errorf("body = %s", s)
	}
	if err := FeatureFile("payments").Decode([]byte(yaml), &body); err == nil || !strings.Contains(err.Error(), "targets payments") {
		t.Errorf("other project err = %v", err)
	}
	if err := FeatureFile("checkout").Decode([]byte("kind: fm/project\nkey: x\n"), &body); err == nil {
		t.Error("wrong kind: expected an error")
	}
}

// fakeFeature is a stateful feature backend that enforces optimistic
// locking and records the writes it receives.
type fakeFeature struct {
	raw    map[string]any
	writes []string // "METHOD path body"
}

func newFakeFeature(t *testing.T) (*fakeFeature, *Handler) {
	t.Helper()
	ff := &fakeFeature{}
	_ = json.Unmarshal([]byte(`{"projectKey":"checkout","key":"new-checkout","name":"New checkout","description":"",
	  "variants":[{"key":"off","name":"Off","displayOrder":0},{"key":"on","name":"On","displayOrder":1}],
	  "flags":[{"key":"layout","name":"Layout","description":"","type":"STRING","variants":{"off":"classic","on":"one-page"},"schema":{"type":"ENUM","values":["classic","one-page","express"]}}],
	  "owner":"","maintainers":[],"status":"ACTIVE","type":"RELEASE","version":3,"links":[{"url":"https://example.invalid/t"}]}`), &ff.raw)
	path := sdkfm.BasePath + "/projects/checkout/features/new-checkout"
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(ff.raw)
			return
		}
		b, _ := io.ReadAll(r.Body)
		ff.writes = append(ff.writes, r.Method+" "+strings.TrimPrefix(r.URL.Path, path)+" "+string(b))
		if v := r.URL.Query().Get("optimistic-locking-version"); r.Method != http.MethodDelete && v != "3" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		var sent map[string]any
		_ = json.Unmarshal(b, &sent)
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPatch:
			ff.raw["status"] = sent["status"]
		case strings.HasSuffix(r.URL.Path, "/metadata"):
			for _, k := range []string{"name", "description", "owner", "maintainers", "type"} {
				ff.raw[k] = sent[k]
			}
		default:
			for k, v := range sent {
				ff.raw[k] = v
			}
		}
		ff.raw["version"] = 4.0
		if r.Method == http.MethodPut && !strings.HasSuffix(r.URL.Path, "/metadata") {
			_ = json.NewEncoder(w).Encode(ff.raw) // the full update returns the feature
			return
		}
		_, _ = w.Write([]byte(`{"version":"4"}`))
	}
	mux.HandleFunc(path, handle)
	mux.HandleFunc(path+"/metadata", handle)
	mux.HandleFunc(path+"/release-stage-targeting", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"releaseStageKey":"staging","enabled":false,"targeting":{"rules":[{"serve":[{"variantKey":"on","percentage":100}]}]}}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return ff, NewHandler(c)
}

func TestChangeFeatureVariantsAndFlags_AddVariantKeepsEverything(t *testing.T) {
	ff, h := newFakeFeature(t)
	res, err := h.ChangeFeatureVariantsAndFlags("checkout", "new-checkout", VariantFlagChange{AddVariant: "express", Values: []string{"layout=express"}}, WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(ff.writes) != 1 || !strings.HasPrefix(ff.writes[0], "PUT  ") {
		t.Fatalf("writes = %v", ff.writes)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(strings.SplitN(ff.writes[0], " ", 3)[2]), &sent)
	if sent["links"] == nil {
		t.Errorf("links dropped: %v", sent)
	}
	flag := sent["flags"].([]any)[0].(map[string]any)
	if flag["schema"] == nil || flag["variants"].(map[string]any)["express"] != "express" {
		t.Errorf("flag = %v", flag)
	}
	if v := sent["variants"].([]any)[2].(map[string]any); v["key"] != "express" || v["displayOrder"] != 2.0 {
		t.Errorf("new variant = %v", v)
	}
}

func TestChangeFeatureVariantsAndFlags_Validation(t *testing.T) {
	ff, h := newFakeFeature(t)
	for _, c := range []VariantFlagChange{
		{AddVariant: "x", Values: []string{"layout=x"}},                  // key too short for the service
		{AddVariant: "Express", Values: []string{"layout=x"}},            // uppercase
		{AddVariant: "express"},                                          // no value for layout
		{AddVariant: "on", Values: []string{"layout=x"}},                 // exists
		{AddFlag: "layout"},                                              // exists
		{AddFlag: "n", FlagType: "NUMBER"},                               // no values: update has no defaults
		{RemoveFlag: "nope"},                                             // missing
		{AddFlag: "n", FlagType: "NUMBER", FlagValues: []string{"on=1"}}, // partial
	} {
		if _, err := h.ChangeFeatureVariantsAndFlags("checkout", "new-checkout", c, WriteOptions{}); err == nil {
			t.Errorf("%+v: expected an error", c)
		}
	}
	if len(ff.writes) != 0 {
		t.Errorf("invalid changes were written: %v", ff.writes)
	}
}

func TestChangeFeatureVariantsAndFlags_RemoveVariantWarnsAboutTargeting(t *testing.T) {
	ff, h := newFakeFeature(t)
	res, err := h.ChangeFeatureVariantsAndFlags("checkout", "new-checkout", VariantFlagChange{RemoveVariant: "on", AddFlag: "beta", FlagValues: []string{"off=true"}}, WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "stage staging still serves removed variant on") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(strings.SplitN(ff.writes[0], " ", 3)[2]), &sent)
	flags := sent["flags"].([]any)
	if _, has := flags[0].(map[string]any)["variants"].(map[string]any)["on"]; has || len(flags) != 2 {
		t.Errorf("flags = %v", flags)
	}
	if beta := flags[1].(map[string]any); beta["type"] != "BOOLEAN" || beta["variants"].(map[string]any)["off"] != true {
		t.Errorf("added flag = %v", beta)
	}
}

func TestFeatureStatusAndMetadata(t *testing.T) {
	ff, h := newFakeFeature(t)
	res, err := h.SetFeatureStatus("checkout", "new-checkout", "ACTIVE", WriteOptions{})
	if err != nil || res.Changed || len(ff.writes) != 0 {
		t.Fatalf("same status: %+v, %v, writes = %v", res, err, ff.writes)
	}
	if res, err = h.SetFeatureStatus("checkout", "new-checkout", "ARCHIVED", WriteOptions{}); err != nil || !res.Changed || res.Feature.Status != "ARCHIVED" {
		t.Fatalf("archive: %+v, %v", res, err)
	}

	ff, h = newFakeFeature(t)
	owner := ""
	if res, err = h.UpdateFeatureMetadata("checkout", "new-checkout", FeatureMetadataChange{Owner: &owner}, WriteOptions{}); err != nil || res.Changed {
		t.Fatalf("same owner: %+v, %v", res, err)
	}
	owner = "checkout-team"
	if res, err = h.UpdateFeatureMetadata("checkout", "new-checkout", FeatureMetadataChange{Owner: &owner}, WriteOptions{}); err != nil || !res.Changed {
		t.Fatalf("owner: %+v, %v", res, err)
	}
	if !strings.Contains(ff.writes[0], `"links"`) || strings.Contains(ff.writes[0], `"status"`) || !strings.Contains(ff.writes[0], "/metadata") {
		t.Errorf("metadata write = %s", ff.writes[0])
	}

	_, h = newFakeFeature(t)
	if _, err := h.SetFeatureStatus("checkout", "new-checkout", "ARCHIVED", WriteOptions{Version: "2"}); err == nil || !strings.Contains(err.Error(), "changed since it was read") {
		t.Errorf("stale version err = %v", err)
	}
}
