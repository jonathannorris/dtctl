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

// fakeApply serves feature new-checkout, its staging targeting with one
// rule (id r1) and the country context attribute, and records writes.
type fakeApply struct {
	writes []string // "METHOD path version body"
	status int      // the write status, 200 unless set
}

func newFakeApply(t *testing.T) (*fakeApply, *Handler) {
	t.Helper()
	fa := &fakeApply{}
	p := sdkfm.BasePath + "/projects/checkout"
	feature := `{"projectKey":"checkout","key":"new-checkout","name":"New checkout","description":"","type":"RELEASE","status":"ACTIVE",
	 "variants":[{"key":"off","name":"Off","displayOrder":0},{"key":"on","name":"On","displayOrder":1}],
	 "flags":[{"key":"new-checkout","name":"new-checkout","description":"","type":"BOOLEAN","variants":{"off":false,"on":true}}],
	 "owner":"","maintainers":[],"links":[{"url":"https://example.invalid/t"}],"version":3}`
	targeting := `{"projectKey":"checkout","featureKey":"new-checkout","releaseStageKey":"staging","enabled":true,"version":5,
	 "targeting":{"rules":[{"id":"r1","serve":[{"variantKey":"on","percentage":100}],
	   "conditions":[{"valueType":"STRING","contextKey":"country","operator":"IS","values":["AT"]}]}]}}`
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, r *http.Request, body string) bool {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(body))
			return false
		}
		b, _ := io.ReadAll(r.Body)
		fa.writes = append(fa.writes, r.Method+" "+strings.TrimPrefix(r.URL.Path, p)+" "+r.URL.Query().Get("optimistic-locking-version")+" "+string(b))
		if fa.status != 0 {
			w.WriteHeader(fa.status)
			return true
		}
		_, _ = w.Write(b)
		return true
	}
	mux.HandleFunc(p+"/features/new-checkout", func(w http.ResponseWriter, r *http.Request) { write(w, r, feature) })
	mux.HandleFunc(p+"/features", func(w http.ResponseWriter, r *http.Request) { write(w, r, `{"items":[]}`) })
	mux.HandleFunc(p+"/features/new-checkout/release-stage-targeting/staging", func(w http.ResponseWriter, r *http.Request) { write(w, r, targeting) })
	mux.HandleFunc(p+"/context-attributes", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"key":"country","name":"Country","type":"STRING","contextKey":"country"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return fa, NewHandler(c)
}

func TestApplyFeature(t *testing.T) {
	fa, h := newFakeApply(t)
	file := "kind: fm/feature\nprojectKey: checkout\nkey: new-checkout\nname: Checkout v2\ndescription: ''\ntype: RELEASE\nstatus: ACTIVE\n" +
		"variants: [{key: off, name: Off, displayOrder: 0}, {key: on, name: On, displayOrder: 1}]\n" +
		"flags: [{key: new-checkout, name: new-checkout, description: '', type: BOOLEAN, variants: {off: false, on: true}}]\n" +
		"owner: ''\nmaintainers: []\nlinks: [{url: 'https://example.invalid/t'}]\nversion: 2\n"

	res, err := h.ApplyFeature("checkout", []byte(file), ApplyOptions{DryRun: true})
	if err != nil || res.Action != ApplyUpdated || !strings.Contains(res.Diff, "Checkout v2") || len(fa.writes) != 0 {
		t.Fatalf("dry run: %+v, %v, writes %v", res, err, fa.writes)
	}
	if res, err = h.ApplyFeature("checkout", []byte(file), ApplyOptions{}); err != nil || res.Action != ApplyUpdated {
		t.Fatalf("apply: %+v, %v", res, err)
	}
	if len(fa.writes) != 1 || !strings.HasPrefix(fa.writes[0], "PUT /features/new-checkout 2 ") || !strings.Contains(fa.writes[0], `"links"`) {
		t.Errorf("write = %v (want the file's version 2, links kept)", fa.writes)
	}

	// The live feature is what the file says: nothing is sent.
	fa, h = newFakeApply(t)
	same := strings.Replace(file, "Checkout v2", "New checkout", 1)
	if res, err = h.ApplyFeature("checkout", []byte(same), ApplyOptions{}); err != nil || res.Action != ApplyUnchanged || len(fa.writes) != 0 {
		t.Errorf("unchanged: %+v, %v, %v", res, err, fa.writes)
	}

	// A flag missing a variant's value is refused before anything is sent.
	partial := strings.Replace(file, "{off: false, on: true}", "{on: true}", 1)
	if _, err = h.ApplyFeature("checkout", []byte(partial), ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "no value for variant(s) off") {
		t.Errorf("partial flag err = %v", err)
	}
	if _, err = h.ApplyFeature("payments", []byte(file), ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "targets payments") {
		t.Errorf("other project err = %v", err)
	}
	fa.status = http.StatusConflict
	if _, err = h.ApplyFeature("checkout", []byte(file), ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "changed since it was read") {
		t.Errorf("conflict err = %v", err)
	}
}

func TestApplyFeature_CreatesAMissingFeature(t *testing.T) {
	fa, h := newFakeApply(t)
	file := "key: other\nname: Other\nvariants: [{key: a, name: A}, {key: b, name: B}]\nflags: [{key: other, type: STRING, variants: {a: x, b: y}}]\nversion: 9\nstatus: ACTIVE\n"
	res, err := h.ApplyFeature("checkout", []byte(file), ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != ApplyCreated || len(fa.writes) != 1 || !strings.HasPrefix(fa.writes[0], "POST /features ") {
		t.Fatalf("create: %+v, writes %v", res, fa.writes)
	}
	if strings.Contains(fa.writes[0], `"version"`) || strings.Contains(fa.writes[0], `"status"`) {
		t.Errorf("create sent read-only members: %s", fa.writes[0])
	}
}

const targetingFile = `projectKey: checkout
featureKey: new-checkout
releaseStageKey: staging
enabled: true
version: 5
targeting:
  rules:
    - serve: [{variantKey: off, percentage: 100}]
      conditions: [{valueType: STRING, contextKey: country, operator: IS, values: [DE]}]
    - id: r1
      serve: [{variantKey: on, percentage: 100}]
      conditions: [{valueType: STRING, contextKey: country, operator: IS, values: [AT]}]
`

func TestApplyFeatureTargeting(t *testing.T) {
	fa, h := newFakeApply(t)
	res, err := h.ApplyFeatureTargeting("checkout", "new-checkout", "staging", []byte(targetingFile), ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != ApplyUpdated || len(fa.writes) != 1 || !strings.HasPrefix(fa.writes[0], "PUT /features/new-checkout/release-stage-targeting/staging 5 ") {
		t.Fatalf("apply: %+v, writes %v", res, fa.writes)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(strings.SplitN(fa.writes[0], " ", 4)[3]), &sent)
	rules := sent["targeting"].(map[string]any)["rules"].([]any)
	if len(rules) != 2 || rules[1].(map[string]any)["id"] != "r1" {
		t.Errorf("rules sent = %v", rules)
	}
	if _, has := sent["version"]; has {
		t.Errorf("the version went in the body: %v", sent)
	}

	// A whole-feature export works; the stage's entry is used.
	fa, h = newFakeApply(t)
	list := "- releaseStageKey: production\n  targeting: {rules: []}\n- " + strings.ReplaceAll(strings.TrimSpace(targetingFile), "\n", "\n  ") + "\n"
	if res, err = h.ApplyFeatureTargeting("checkout", "new-checkout", "staging", []byte(list), ApplyOptions{DryRun: true}); err != nil || res.Action != ApplyUpdated || len(fa.writes) != 0 {
		t.Errorf("list dry run: %+v, %v, %v", res, err, fa.writes)
	}
}

func TestApplyFeatureTargeting_Checks(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"unknown variant":     {"variantKey: off", "variantKey: maybe", `serves "maybe"`},
		"split not 100":       {"{variantKey: off, percentage: 100}", "{variantKey: off, percentage: 60}", "total"},
		"unknown context key": {"contextKey: country, operator: IS, values: [DE]", "contextKey: plan, operator: IS, values: [DE]", "not a context attribute"},
		"wrong value type":    {"valueType: STRING, contextKey: country, operator: IS, values: [DE]", "valueType: NUMBER, contextKey: country, operator: IS, values: [DE]", "valueType is NUMBER"},
		"unsupported op":      {"operator: IS, values: [DE]", "operator: GREATER_THAN, values: [DE]", "does not apply"},
		"dropped rule id":     {"    - id: r1\n", "    - name: kept\n", "re-buckets"},
		"invented rule id":    {"    - serve: [{variantKey: off", "    - id: r9\n      serve: [{variantKey: off", "does not have"},
		"other stage":         {"releaseStageKey: staging", "releaseStageKey: production", "targets staging"},
	} {
		fa, h := newFakeApply(t)
		file := strings.Replace(targetingFile, tc.from, tc.to, 1)
		if file == targetingFile {
			t.Fatalf("%s: the replacement did not apply", name)
		}
		_, err := h.ApplyFeatureTargeting("checkout", "new-checkout", "staging", []byte(file), ApplyOptions{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if len(fa.writes) != 0 {
			t.Errorf("%s: a refused file was written: %v", name, fa.writes)
		}
	}

	fa, h := newFakeApply(t)
	dropped := strings.Replace(targetingFile, "    - id: r1\n", "    - name: kept\n", 1)
	if _, err := h.ApplyFeatureTargeting("checkout", "new-checkout", "staging", []byte(dropped), ApplyOptions{Force: true}); err != nil || len(fa.writes) != 1 {
		t.Errorf("--force: %v, writes %v", err, fa.writes)
	}
}
