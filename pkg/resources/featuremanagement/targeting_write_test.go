package featuremanagement

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

var testNow = time.Date(2026, 11, 24, 21, 0, 0, 0, time.UTC)

func TestParseTimeSpec(t *testing.T) {
	tests := []struct {
		in, want, wantErr string
	}{
		{in: "now", want: "2026-11-24T21:00:00Z"},
		{in: "+30m", want: "2026-11-24T21:30:00Z"},
		{in: "+1d", want: "2026-11-25T21:00:00Z"},
		{in: "2026-12-01T09:00Z", want: "2026-12-01T09:00:00Z"},
		{in: "2026-12-01T10:00+01:00", want: "2026-12-01T09:00:00Z"},
		{in: "2026-12-01T09:00:30Z", want: "2026-12-01T09:00:30Z"},
		{in: "1796115600", want: "2026-12-01T09:00:00Z"},
		{in: "1796115600000", wantErr: "looks like milliseconds"},
		{in: "2026-12-01T09:00", wantErr: "has no zone"},
		{in: "tomorrow", wantErr: "invalid time"},
	}
	for _, tt := range tests {
		got, err := ParseTimeSpec(tt.in, testNow)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseTimeSpec(%q) err = %v, want %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got.Format(time.RFC3339) != tt.want {
			t.Errorf("ParseTimeSpec(%q) = %v, %v; want %s", tt.in, got, err, tt.want)
		}
	}
}

func TestParseConditionExpr(t *testing.T) {
	for _, bad := range []struct{ in, wantErr string }{
		{"targetingKey", "want '<context-key> <operator> <values>'"},
		{"targetingKey == a", "unknown operator"},
		{"plan empty x", "takes no values"},
		{"plan =", "needs at least one value"},
		{"d between 1", "takes 2 value(s)"},
		{"n > 1,2", "takes 1 value(s)"},
		{"k = a,,b", "empty value"},
	} {
		if _, err := ParseConditionExpr(bad.in); err == nil || !strings.Contains(err.Error(), bad.wantErr) {
			t.Errorf("ParseConditionExpr(%q) err = %v, want %q", bad.in, err, bad.wantErr)
		}
	}
	e, err := ParseConditionExpr("targetingKey = user-1, user-2")
	if err != nil || e.ContextKey != "targetingKey" || e.Operator != "=" || strings.Join(e.Values, "|") != "user-1|user-2" {
		t.Errorf("parsed = %+v, %v", e, err)
	}
	// Symbolic operators need no spaces; the two-character ones win.
	for _, tt := range []struct{ in, key, op, values string }{
		{"targetingKey=user1,user2,user3", "targetingKey", "=", "user1|user2|user3"},
		{"country!=us", "country", "!=", "us"},
		{"app.version>=2.3.0", "app.version", ">=", "2.3.0"},
		{"n<=5", "n", "<=", "5"},
		{"n < 5", "n", "<", "5"},
		{"app.version^1.2.0", "app.version", "^", "1.2.0"},
		{"plan.tier= pro, enterprise", "plan.tier", "=", "pro|enterprise"},
	} {
		e, err := ParseConditionExpr(tt.in)
		if err != nil || e.ContextKey != tt.key || e.Operator != tt.op || strings.Join(e.Values, "|") != tt.values {
			t.Errorf("ParseConditionExpr(%q) = %+v, %v", tt.in, e, err)
		}
	}
	if _, err := ParseConditionExpr("countrycontainsus"); err == nil {
		t.Error("a word operator without spaces must not parse")
	}
}

func TestTypedCondition(t *testing.T) {
	types := ContextKeyTypes{"n": "NUMBER", "b": "BOOLEAN", "d": "DATE_TIME", "v": "SEM_VER", "s": "STRING"}
	tests := []struct {
		expr    string
		want    string // JSON of the condition
		wantErr string
	}{
		{expr: "targetingKey = u1,u2", want: `{"valueType":"STRING","contextKey":"targetingKey","operator":"IS","values":["u1","u2"]}`},
		{expr: "n >= 2.5", want: `{"valueType":"NUMBER","contextKey":"n","operator":"GREATER_THAN_OR_EQUAL","values":[2.5]}`},
		{expr: "b = true", want: `{"valueType":"BOOLEAN","contextKey":"b","operator":"IS","values":[true]}`},
		{expr: "d after 2026-01-01T00:00Z", want: `{"valueType":"DATE_TIME","contextKey":"d","operator":"AFTER","values":["1767225600"]}`},
		{expr: "v ^ 2.0.0", want: `{"valueType":"SEM_VER","contextKey":"v","operator":"MATCHES_MAJOR","values":["2.0.0"]}`},
		{expr: "s exists", want: `{"valueType":"STRING","contextKey":"s","operator":"EXISTS"}`},
		{expr: "n contains 1", wantErr: "operator contains does not apply to n (a NUMBER)"},
		{expr: "b = maybe", wantErr: `"maybe" is not true or false`},
		{expr: "n = x", wantErr: `"x" is not a number`},
		{expr: "nope = x", wantErr: `context key "nope" is not a context attribute of project "checkout"`},
	}
	for _, tt := range tests {
		e, err := ParseConditionExpr(tt.expr)
		if err != nil {
			t.Fatalf("ParseConditionExpr(%q): %v", tt.expr, err)
		}
		c, err := types.TypedCondition("checkout", e, testNow)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("TypedCondition(%q) err = %v, want %q", tt.expr, err, tt.wantErr)
			}
			continue
		}
		got, _ := json.Marshal(c)
		if err != nil || string(got) != tt.want {
			t.Errorf("TypedCondition(%q) = %s, %v; want %s", tt.expr, got, err, tt.want)
		}
	}
}

func TestParseServe(t *testing.T) {
	variants := []Variant{{Key: "baseline"}, {Key: "variation-a"}}
	if got, err := ParseServe("variation-a", variants); err != nil || len(got) != 1 || got[0].Percentage != 100 {
		t.Errorf("single = %+v, %v", got, err)
	}
	if got, err := ParseServe("baseline=50,variation-a=50", variants); err != nil || len(got) != 2 {
		t.Errorf("split = %+v, %v", got, err)
	}
	for _, bad := range []struct{ in, wantErr string }{
		{"other", `unknown variant "other": the feature's variants are baseline, variation-a`},
		{"baseline=60,variation-a=50", "totals 110%"},
		{"baseline=50,baseline=50", "appears twice"},
		{"baseline=x", "must be a number from 0 to 100"},
	} {
		if _, err := ParseServe(bad.in, variants); err == nil || !strings.Contains(err.Error(), bad.wantErr) {
			t.Errorf("ParseServe(%q) err = %v, want %q", bad.in, err, bad.wantErr)
		}
	}
}

func TestParseRollout(t *testing.T) {
	r, err := ParseRollout([]string{"now=10", "+1d=50,linear", "2026-11-27T21:00Z=100"}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) != 3 || r.Steps[1].Transition != sdkfm.RolloutTransitionLinear || !r.Steps[0].EffectiveTime.Equal(testNow) || r.Steps[2].Percentage != 100 {
		t.Errorf("steps = %+v", r.Steps)
	}
	for _, bad := range []struct {
		in      []string
		wantErr string
	}{
		{[]string{"now"}, "want <time>=<percent>"},
		{[]string{"+1d=10", "now=20"}, "time order"},
		{[]string{"now=10,smooth"}, "must be linear or step"},
		{[]string{"now=120"}, "from 0 to 100"},
	} {
		if _, err := ParseRollout(bad.in, testNow); err == nil || !strings.Contains(err.Error(), bad.wantErr) {
			t.Errorf("ParseRollout(%q) err = %v, want %q", bad.in, err, bad.wantErr)
		}
	}
}

func TestResolveRulePosition(t *testing.T) {
	rules := []TargetingRule{{ID: "6f1c2b9e-1"}, {ID: "a93d07e1-2"}, {ID: "a93d0999-3"}}
	tests := []struct {
		in      string
		insert  bool
		want    int
		wantErr string
	}{
		{in: "0", want: 0},
		{in: "2", want: 2},
		{in: "3", wantErr: "positions are 0 to 2"},
		{in: "3", insert: true, want: 3},
		{in: "6F1C", want: 0},
		{in: "a93d07e1", want: 1},
		{in: "a93d0", wantErr: "ambiguous"},
		{in: "ffff", wantErr: `no rule has id "ffff"`},
	}
	for _, tt := range tests {
		got, err := ResolveRulePosition(rules, tt.in, tt.insert)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ResolveRulePosition(%q) err = %v, want %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ResolveRulePosition(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
}

// fakeStage is a stateful Feature Management backend holding one feature's
// targeting in one stage. It enforces optimistic locking like the service.
type fakeStage struct {
	t        *testing.T
	ft       sdkfm.FeatureTargetingDetails
	puts     []map[string]any
	conflict int // answer this many PUTs with 409 after moving the version on
	attrs    []sdkfm.ContextAttributeDetails
}

const writeFeaturePath = sdkfm.BasePath + "/projects/checkout/features/new-checkout"

func newFakeStage(t *testing.T, rules string) (*fakeStage, *Handler) {
	t.Helper()
	fs := &fakeStage{t: t, attrs: []sdkfm.ContextAttributeDetails{
		{Key: "account-id", ContextKey: "account.id", Type: "STRING"},
		{Key: "app-version", ContextKey: "app.version", Type: "SEM_VER"},
	}}
	raw := `{"projectKey":"checkout","featureKey":"new-checkout","releaseStageKey":"production","enabled":true,"version":4,"targeting":{"rules":` + rules + `}}`
	if err := json.Unmarshal([]byte(raw), &fs.ft); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(writeFeaturePath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureDetails{ProjectKey: "checkout", Key: "new-checkout",
			Variants: []sdkfm.Variant{{Key: "baseline"}, {Key: "variation-a"}}})
	})
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/context-attributes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.ContextAttributeList{Items: fs.attrs})
	})
	mux.HandleFunc(writeFeaturePath+"/release-stage-targeting/production", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(fs.ft)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var sent map[string]any
			_ = json.Unmarshal(body, &sent)
			fs.puts = append(fs.puts, sent)
			if fs.conflict > 0 {
				fs.conflict--
				fs.ft.Version++
			}
			if r.URL.Query().Get("optimistic-locking-version") != strconv.Itoa(fs.ft.Version) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":{"code":409,"message":"Optimistic locking failed"}}`))
				return
			}
			var upd sdkfm.FeatureTargetingUpdate
			_ = json.Unmarshal(body, &upd)
			for i := range upd.Targeting.Rules {
				if upd.Targeting.Rules[i].ID == "" {
					upd.Targeting.Rules[i].ID = "new00000-" + strconv.Itoa(len(fs.puts))
				}
			}
			fs.ft.Enabled, fs.ft.Targeting = upd.Enabled, upd.Targeting
			fs.ft.Version++
			_ = json.NewEncoder(w).Encode(fs.ft)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return fs, NewHandler(c)
}

const twoRules = `[
 {"id":"6f1c2b9e-0001","conditions":[{"valueType":"STRING","contextKey":"targetingKey","operator":"IS","values":["user-1","user-2"]}],
  "serve":[{"variantKey":"variation-a","percentage":100}],"futureField":"kept"},
 {"id":"a93d07e1-0002","conditions":[],"serve":[{"variantKey":"baseline","percentage":100}]}]`

func str(s string) *string { return &s }

func rulesOf(t *testing.T, put map[string]any) []map[string]any {
	t.Helper()
	targeting, _ := put["targeting"].(map[string]any)
	list, _ := targeting["rules"].([]any)
	out := make([]map[string]any, len(list))
	for i, r := range list {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func TestCreateTargetingRule_InsertsKeepingIDsAndUnknownFields(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	res, err := h.CreateTargetingRule("checkout", "new-checkout", "production", RuleSpec{
		Position:   "0",
		Variant:    str("baseline=50,variation-a=50"),
		Conditions: []string{"account.id = acct-1,acct-2", "app.version >= 2.3.0"},
		Rollout:    []string{"now=10", "+7d=100,linear"},
	}, TargetingWriteOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Position != 0 || res.RuleID == "" {
		t.Errorf("result = %+v", res)
	}
	if len(fs.puts) != 1 {
		t.Fatalf("puts = %d", len(fs.puts))
	}
	rules := rulesOf(t, fs.puts[0])
	if len(rules) != 3 {
		t.Fatalf("rules = %v", rules)
	}
	if _, has := rules[0]["id"]; has {
		t.Errorf("new rule was sent with an id: %v", rules[0])
	}
	if rules[1]["id"] != "6f1c2b9e-0001" || rules[1]["futureField"] != "kept" || rules[2]["id"] != "a93d07e1-0002" {
		t.Errorf("existing rules changed: %v", rules[1:])
	}
	if fs.puts[0]["enabled"] != true {
		t.Errorf("enabled = %v", fs.puts[0]["enabled"])
	}
}

func TestCreateTargetingRule_ValidatesBeforeWriting(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	for _, spec := range []RuleSpec{
		{Variant: str("nope")},
		{Variant: str("baseline"), Conditions: []string{"plan.tier = pro"}},
		{Variant: str("baseline"), Position: "9"},
		{Variant: str("baseline"), BucketingKey: str("org.id")},
	} {
		if _, err := h.CreateTargetingRule("checkout", "new-checkout", "production", spec, TargetingWriteOptions{Now: testNow}); err == nil {
			t.Errorf("spec %+v: expected an error", spec)
		}
	}
	if len(fs.puts) != 0 {
		t.Errorf("an invalid rule was written: %v", fs.puts)
	}
}

func TestUpdateTargetingRule_ConditionAddRemove(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	res, err := h.UpdateTargetingRule("checkout", "new-checkout", "production", RuleSpec{
		Position:        "6f1c",
		ConditionAdd:    []string{"targetingKey = user-3,user-2"},
		ConditionRemove: []string{"targetingKey = user-1"},
	}, TargetingWriteOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Position != 0 || res.RuleID != "6f1c2b9e-0001" {
		t.Errorf("result = %+v", res)
	}
	cond := rulesOf(t, fs.puts[0])[0]["conditions"].([]any)[0].(map[string]any)
	if got := cond["values"].([]any); len(got) != 2 || got[0] != "user-2" || got[1] != "user-3" {
		t.Errorf("values = %v, want [user-2 user-3]", got)
	}
}

func TestUpdateTargetingRule_RemovingLastValueRemovesCondition(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	if _, err := h.UpdateTargetingRule("checkout", "new-checkout", "production", RuleSpec{
		Position:        "0",
		ConditionRemove: []string{"targetingKey = user-1,user-2"},
	}, TargetingWriteOptions{Now: testNow}); err != nil {
		t.Fatal(err)
	}
	if conds := rulesOf(t, fs.puts[0])[0]["conditions"]; conds != nil {
		t.Errorf("conditions = %v, want none", conds)
	}
}

func TestUpdateTargetingRule_MoveAndShadowWarning(t *testing.T) {
	_, h := newFakeStage(t, twoRules)
	res, err := h.UpdateTargetingRule("checkout", "new-checkout", "production", RuleSpec{Position: "1", MovePosition: "0"}, TargetingWriteOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if res.Position != 0 || res.Targeting.Targeting.Rules[0].ID != "a93d07e1-0002" {
		t.Errorf("result = %+v", res)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "rule 1 will not be evaluated: rule 0 matches all requests" {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestUpdateTargetingRule_RollbackIsANoOpWhenAlreadyAtZero(t *testing.T) {
	fs, h := newFakeStage(t, `[{"id":"r1","serve":[{"variantKey":"baseline","percentage":100}],
	  "rollout":{"steps":[{"effectiveTime":"2026-11-20T00:00:00Z","percentage":0}]}}]`)
	res, err := h.UpdateTargetingRule("checkout", "new-checkout", "production", RuleSpec{Position: "r1", Rollout: []string{"now=0"}}, TargetingWriteOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || len(fs.puts) != 0 {
		t.Errorf("changed = %v, puts = %d; want a no-op", res.Changed, len(fs.puts))
	}
}

func TestSetTargetingEnabled(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	res, err := h.SetTargetingEnabled("checkout", "new-checkout", "production", true, TargetingWriteOptions{})
	if err != nil || res.Changed || len(fs.puts) != 0 {
		t.Fatalf("enable an enabled stage: changed = %v, puts = %d, err = %v", res != nil && res.Changed, len(fs.puts), err)
	}
	res, err = h.SetTargetingEnabled("checkout", "new-checkout", "production", false, TargetingWriteOptions{})
	if err != nil || !res.Changed {
		t.Fatalf("disable: %+v, %v", res, err)
	}
	if fs.puts[0]["enabled"] != false || len(rulesOf(t, fs.puts[0])) != 2 {
		t.Errorf("sent = %v", fs.puts[0])
	}
}

func TestClearAndDeleteTargetingRules(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	res, err := h.DeleteTargetingRule("checkout", "new-checkout", "production", "a93d", TargetingWriteOptions{})
	if err != nil || res.RuleID != "a93d07e1-0002" || res.Removed != 1 {
		t.Fatalf("delete: %+v, %v", res, err)
	}
	res, err = h.ClearTargetingRules("checkout", "new-checkout", "production", TargetingWriteOptions{})
	if err != nil || res.Removed != 1 || !res.Changed {
		t.Fatalf("clear: %+v, %v", res, err)
	}
	last := fs.puts[len(fs.puts)-1]
	if fs, ok := last["targeting"].(map[string]any)["rules"].([]any); !ok || len(fs) != 0 {
		t.Errorf("rules sent = %v, want an empty list", last["targeting"])
	}
	if last["enabled"] != true {
		t.Errorf("clear changed enabled: %v", last["enabled"])
	}
	res, err = h.ClearTargetingRules("checkout", "new-checkout", "production", TargetingWriteOptions{})
	if err != nil || res.Changed {
		t.Errorf("clearing an empty stage: %+v, %v", res, err)
	}
}

func TestWriteTargeting_RetriesOnceOnConflict(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	fs.conflict = 1
	res, err := h.SetTargetingEnabled("checkout", "new-checkout", "production", false, TargetingWriteOptions{})
	if err != nil || !res.Changed || len(fs.puts) != 2 {
		t.Fatalf("result = %+v, err = %v, puts = %d; want a retried write", res, err, len(fs.puts))
	}

	fs, h = newFakeStage(t, twoRules)
	fs.conflict = 2
	_, err = h.SetTargetingEnabled("checkout", "new-checkout", "production", false, TargetingWriteOptions{})
	if !errors.Is(err, httpclient.ErrConflict) || !strings.Contains(err.Error(), "changed since it was read; re-run to retry") {
		t.Errorf("err = %v, want a conflict after one retry", err)
	}
	if len(fs.puts) != 2 {
		t.Errorf("puts = %d, want 2", len(fs.puts))
	}
}

func TestWriteTargeting_ExplicitVersionIsNotRetried(t *testing.T) {
	fs, h := newFakeStage(t, twoRules)
	_, err := h.SetTargetingEnabled("checkout", "new-checkout", "production", false, TargetingWriteOptions{Version: "3"})
	if !errors.Is(err, httpclient.ErrConflict) || len(fs.puts) != 1 {
		t.Errorf("err = %v, puts = %d; want one 409", err, len(fs.puts))
	}
}
