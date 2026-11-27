package featuremanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

const targetingJSON = `{"projectKey":"checkout","featureKey":"new-checkout","releaseStageKey":"production","enabled":true,
 "targeting":{"rules":[
  {"id":"6f1c2b9e-0000-4000-8000-000000000001","serve":[{"variantKey":"on","percentage":100}],
   "conditions":[{"valueType":"STRING","contextKey":"targetingKey","operator":"IS","values":["user-1","user-2"]},
                 {"valueType":"DATE_TIME","contextKey":"signup","operator":"AFTER","values":["1767225600"]}]},
  {"id":"a93d07e1-0000-4000-8000-000000000002","name":"Everyone else","serve":[{"variantKey":"on","percentage":100}],
   "rollout":{"steps":[{"effectiveTime":"2026-11-24T21:00:00Z","percentage":10},
                       {"effectiveTime":"2026-11-25T21:00:00Z","percentage":50,"transition":"LINEAR"}]},
   "bucketingKey":"account.id"}]},
 "version":6,"modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z","lastModifiedBy":"u","lastModifiedTime":"2026-11-03T10:00:00Z"}}`

func TestGetFeatureTargeting_DecodesRules(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(targetingJSON))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	ft, err := NewHandler(c).GetFeatureTargeting(context.Background(), "checkout", "a/b", "prod:x")
	if err != nil {
		t.Fatalf("GetFeatureTargeting: %v", err)
	}
	if want := BasePath + "/projects/checkout/features/a%2Fb/release-stage-targeting/prod%3Ax"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	rules := ft.Targeting.Rules
	if !ft.Enabled || ft.Version != 6 || len(rules) != 2 {
		t.Fatalf("decoded = %+v", ft)
	}
	if c := rules[0].Conditions; len(c) != 2 || c[0].Operator != "IS" || c[0].Values[1] != "user-2" || c[1].ValueType != ConditionValueTypeDateTime {
		t.Errorf("conditions = %+v", c)
	}
	steps := rules[1].Rollout.Steps
	if len(steps) != 2 || steps[1].Transition != RolloutTransitionLinear || !steps[0].EffectiveTime.Equal(time.Date(2026, 11, 24, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("rollout = %+v", steps)
	}
	if rules[1].BucketingKey != "account.id" || rules[1].Name != "Everyone else" {
		t.Errorf("rule = %+v", rules[1])
	}
}

func TestListFeatureTargeting_Path(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"items":[` + targetingJSON + `]}`))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewHandler(c).ListFeatureTargeting(context.Background(), "checkout", "new-checkout", ListOptions{})
	if err != nil {
		t.Fatalf("ListFeatureTargeting: %v", err)
	}
	if want := BasePath + "/projects/checkout/features/new-checkout/release-stage-targeting"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if len(list.Items) != 1 || list.Items[0].ReleaseStageKey != "production" {
		t.Errorf("items = %+v", list.Items)
	}
}

func TestTargetingRoundTripKeepsUnknownMembers(t *testing.T) {
	in := `{"rules":[{"id":"r1","futureRuleField":{"a":1},"serve":[{"variantKey":"on","percentage":100,"weightHint":3}],
	  "conditions":[{"valueType":"STRING","contextKey":"k","operator":"IS","values":["x"],"caseInsensitive":true}],
	  "rollout":{"steps":[{"effectiveTime":"2026-11-24T21:00:00Z","percentage":10,"jitter":"5m"}],"hold":false}}],
	  "defaultServe":"off"}`
	var tg Targeting
	if err := json.Unmarshal([]byte(in), &tg); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(tg)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	_ = json.Unmarshal(out, &got)
	_ = json.Unmarshal([]byte(in), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip lost members:\n got %s\nwant %s", out, in)
	}
}

func TestTargetingWithoutUnknownMembersKeepsFieldOrder(t *testing.T) {
	r := TargetingRule{ID: "r1", Name: "n", Serve: []VariantDistribution{{VariantKey: "on", Percentage: 100}}}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"r1","name":"n","serve":[{"variantKey":"on","percentage":100}]}`; string(out) != want {
		t.Errorf("got %s, want %s", out, want)
	}
	empty, _ := json.Marshal(Targeting{})
	if string(empty) != `{"rules":[]}` {
		t.Errorf("empty targeting = %s, want rules as an empty list", empty)
	}
}

func TestUpdateFeatureTargeting_SendsVersionAndBody(t *testing.T) {
	var method, path, version string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.EscapedPath()
		version = r.URL.Query().Get("optimistic-locking-version")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(targetingJSON))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	ft, err := NewHandler(c).UpdateFeatureTargeting(context.Background(), "checkout", "new-checkout", "production",
		FeatureTargetingUpdate{Enabled: false}, "6")
	if err != nil {
		t.Fatalf("UpdateFeatureTargeting: %v", err)
	}
	if method != http.MethodPut || version != "6" {
		t.Errorf("method = %s, version = %q", method, version)
	}
	if want := BasePath + "/projects/checkout/features/new-checkout/release-stage-targeting/production"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if body["enabled"] != false || fmt.Sprint(body["targeting"]) != "map[rules:[]]" {
		t.Errorf("body = %v", body)
	}
	if ft.Version != 6 {
		t.Errorf("result = %+v", ft)
	}
}

func TestUpdateFeatureTargeting_ConflictIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":409,"message":"Optimistic locking failed"}}`))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHandler(c).UpdateFeatureTargeting(context.Background(), "checkout", "f", "production", FeatureTargetingUpdate{}, "3")
	if !errors.Is(err, httpclient.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}
