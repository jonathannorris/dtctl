package featuremanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
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
