package featuremanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

func TestFormatCondition(t *testing.T) {
	tests := []struct {
		c    TargetingCondition
		want string
	}{
		{TargetingCondition{ValueType: "STRING", ContextKey: "targetingKey", Operator: "IS", Values: []any{"key1", "key2"}}, "targetingKey = key1,key2"},
		{TargetingCondition{ValueType: "NUMBER", ContextKey: "num", Operator: "IS", Values: []any{610.0}}, "num = 610"},
		{TargetingCondition{ValueType: "NUMBER", ContextKey: "ratio", Operator: "LESS_THAN_OR_EQUAL", Values: []any{0.25}}, "ratio <= 0.25"},
		{TargetingCondition{ValueType: "SEM_VER", ContextKey: "semver", Operator: "GREATER_THAN_OR_EQUAL", Values: []any{"1.1.1"}}, "semver >= 1.1.1"},
		{TargetingCondition{ValueType: "SEM_VER", ContextKey: "app.version", Operator: "MATCHES_MAJOR", Values: []any{"2.0.0"}}, "app.version ^ 2.0.0"},
		{TargetingCondition{ValueType: "BOOLEAN", ContextKey: "beta.optin", Operator: "IS", Values: []any{true}}, "beta.optin = true"},
		{TargetingCondition{ValueType: "DATE_TIME", ContextKey: "date", Operator: "BEFORE", Values: []any{"1793682000"}}, "date before 2026-11-03T05:00Z"},
		{TargetingCondition{ValueType: "DATE_TIME", ContextKey: "d", Operator: "BETWEEN", Values: []any{"1793682000", "1793682001"}}, "d between 2026-11-03T05:00Z,2026-11-03T05:00:01Z"},
		{TargetingCondition{ValueType: "STRING", ContextKey: "plan", Operator: "IS_NOT_EMPTY"}, "plan !empty"},
		{TargetingCondition{ValueType: "STRING", ContextKey: "plan", Operator: "DOES_NOT_EXIST"}, "plan !exists"},
		{TargetingCondition{ValueType: "STRING", ContextKey: "plan", Operator: "FUZZY", Values: []any{"x"}}, "plan FUZZY x"},
	}
	for _, tt := range tests {
		if got := FormatCondition(tt.c); got != tt.want {
			t.Errorf("FormatCondition(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestFormatServeAndRollout(t *testing.T) {
	if got := FormatServe([]VariantDistribution{{VariantKey: "on", Percentage: 100}}); got != "on" {
		t.Errorf("single = %q", got)
	}
	if got := FormatServe([]VariantDistribution{{VariantKey: "a", Percentage: 50}, {VariantKey: "b", Percentage: 50}}); got != "a=50,b=50" {
		t.Errorf("split = %q", got)
	}
	r := &Rollout{Steps: []RolloutStep{
		{Percentage: 10}, {Percentage: 50, Transition: "LINEAR"}, {Percentage: 100, Transition: "STEP"},
	}}
	if got := FormatRolloutSummary(r); got != "10% -> 50% (linear) -> 100%" {
		t.Errorf("rollout = %q", got)
	}
	if got := FormatRolloutSummary(nil); got != "-" {
		t.Errorf("no rollout = %q", got)
	}
	step := RolloutStep{EffectiveTime: time.Date(2026, 11, 24, 21, 0, 0, 0, time.UTC), Percentage: 12.5, Transition: "LINEAR"}
	if got := FormatRolloutStep(step); got != "12.5% at 2026-11-24 21:00Z (linear)" {
		t.Errorf("step = %q", got)
	}
}

func TestShadowedRules(t *testing.T) {
	cond := []TargetingCondition{{ContextKey: "k", Operator: "IS", Values: []any{"v"}}}
	got := ShadowedRules([]TargetingRule{{Conditions: cond}, {}, {Conditions: cond}, {}})
	want := []string{
		"rule 2 will not be evaluated: rule 1 matches all requests",
		"rule 3 will not be evaluated: rule 1 matches all requests",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ShadowedRules = %q, want %q", got, want)
	}
	// A rollout rule matches only its share of requests; the rest fall
	// through, so the rule below it is still evaluated.
	rollout := &Rollout{Steps: []RolloutStep{{Percentage: 10}}}
	if got := ShadowedRules([]TargetingRule{{Rollout: rollout}, {}}); got != nil {
		t.Errorf("rollout rule: ShadowedRules = %q, want none", got)
	}
	if got := ShadowedRules([]TargetingRule{{Conditions: cond}, {}}); got != nil {
		t.Errorf("catch-all last = %q, want none", got)
	}
}

func TestTargetingRows(t *testing.T) {
	rows := TargetingRows([]FeatureTargeting{
		{ReleaseStageKey: "development", Version: 1},
		{ReleaseStageKey: "production", Enabled: true, Version: 6, Targeting: Targeting{Rules: []TargetingRule{
			{ID: "6f1c2b9e-0000-4000-8000-000000000001", Serve: []VariantDistribution{{VariantKey: "on", Percentage: 100}},
				Conditions: []TargetingCondition{
					{ValueType: "STRING", ContextKey: "targetingKey", Operator: "IS", Values: []any{"u1"}},
					{ValueType: "NUMBER", ContextKey: "n", Operator: "GREATER_THAN", Values: []any{3.0}},
				}},
			{ID: "a93d07e1-0000-4000-8000-000000000002", Serve: []VariantDistribution{{VariantKey: "on", Percentage: 100}}},
		}}},
	})
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Targeting != "off" || rows[0].Conditions != "(no rules)" || rows[0].Position != "-" {
		t.Errorf("empty stage row = %+v", rows[0])
	}
	if rows[1].Position != "0" || rows[1].ID != "6f1c2b9e" || rows[1].Conditions != "targetingKey = u1 and n > 3" {
		t.Errorf("rule row = %+v", rows[1])
	}
	if rows[2].Conditions != AllRequests || rows[2].Targeting != "on" {
		t.Errorf("catch-all row = %+v", rows[2])
	}
}

// fakeTargeting serves one feature's per-stage targeting, the project's
// release stages (unless stagesForbidden) and its features, for the
// key-or-name and stage-order tests.
func fakeTargeting(t *testing.T, stagesForbidden bool) *Handler {
	t.Helper()
	feature := sdkfm.FeatureDetails{Key: "new-checkout", Name: "New checkout"}
	targeting := []sdkfm.FeatureTargetingDetails{
		{ReleaseStageKey: "production", Enabled: true},
		{ReleaseStageKey: "development"},
		{ReleaseStageKey: "staging"},
	}
	stages := []sdkfm.ReleaseStageDetails{
		{Key: "development", DisplayOrder: 0}, {Key: "staging", DisplayOrder: 1}, {Key: "production", DisplayOrder: 2},
	}
	base := sdkfm.BasePath + "/projects/checkout"
	notFound := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
	}
	mux := http.NewServeMux()
	mux.HandleFunc(base+"/release-stages", func(w http.ResponseWriter, r *http.Request) {
		if stagesForbidden {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"missing scope"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(sdkfm.ReleaseStageList{Items: stages})
	})
	mux.HandleFunc(base+"/features", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureList{Items: []sdkfm.FeatureDetails{feature}})
	})
	mux.HandleFunc(base+"/features/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, base+"/features/")
		key, sub, _ := strings.Cut(rest, "/")
		if !keyPattern.MatchString(key) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
			return
		}
		if key != feature.Key {
			notFound(w)
			return
		}
		switch {
		case sub == "":
			_ = json.NewEncoder(w).Encode(feature)
		case sub == "release-stage-targeting":
			_ = json.NewEncoder(w).Encode(sdkfm.FeatureTargetingList{Items: targeting})
		case strings.HasPrefix(sub, "release-stage-targeting/"):
			stage := strings.TrimPrefix(sub, "release-stage-targeting/")
			for _, ft := range targeting {
				if ft.ReleaseStageKey == stage {
					_ = json.NewEncoder(w).Encode(ft)
					return
				}
			}
			notFound(w)
		default:
			notFound(w)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(c)
}

func stageKeys(items []FeatureTargeting) string {
	keys := make([]string, len(items))
	for i, ft := range items {
		keys[i] = ft.ReleaseStageKey
	}
	return strings.Join(keys, ",")
}

func TestListFeatureTargeting_StageOrderAndName(t *testing.T) {
	h := fakeTargeting(t, false)
	items, err := h.ListFeatureTargeting("checkout", "New checkout")
	if err != nil {
		t.Fatal(err)
	}
	if got := stageKeys(items); got != "development,staging,production" {
		t.Errorf("stage order = %s", got)
	}
}

// Without the release-stages scope the command still works, ordered by key.
func TestListFeatureTargeting_NoStageScope(t *testing.T) {
	h := fakeTargeting(t, true)
	items, err := h.ListFeatureTargeting("checkout", "new-checkout")
	if err != nil {
		t.Fatal(err)
	}
	if got := stageKeys(items); got != "development,production,staging" {
		t.Errorf("stage order = %s", got)
	}
}

func TestGetFeatureTargeting(t *testing.T) {
	h := fakeTargeting(t, false)
	ft, err := h.GetFeatureTargeting("checkout", "New checkout", "production")
	if err != nil {
		t.Fatal(err)
	}
	if !ft.Enabled || ft.ReleaseStageKey != "production" {
		t.Errorf("targeting = %+v", ft)
	}
	if _, err := h.GetFeatureTargeting("checkout", "nope", "production"); err == nil || !strings.Contains(err.Error(), `feature "nope" not found`) {
		t.Errorf("unknown feature error = %v", err)
	}
}
