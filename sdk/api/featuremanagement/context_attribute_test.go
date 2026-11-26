package featuremanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func TestContextAttributes(t *testing.T) {
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		switch r.URL.EscapedPath() {
		case BasePath + "/projects/checkout/context-attributes":
			_, _ = w.Write([]byte(`{"items":[{"key":"plan-tier","name":"Plan tier","description":"","schema":"{\"type\":\"string\"}","type":"STRING","contextKey":"plan.tier","projectKey":"checkout","version":1,"usageCount":3,"builtIn":false,"modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z","lastModifiedBy":"u","lastModifiedTime":"2026-11-03T10:00:00Z"}}]}`))
		case BasePath + "/projects/checkout/context-attributes/$key%3Ax/usage":
			_, _ = w.Write([]byte(`{"items":[{"featureKey":"new-checkout","featureName":"New checkout","releaseStages":[{"releaseStageKey":"production","releaseStageName":"Production"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(c)
	list, err := h.ListContextAttributes(context.Background(), "checkout", ListOptions{AddFields: []string{AddFieldUsagesCount}})
	if err != nil {
		t.Fatalf("ListContextAttributes: %v", err)
	}
	a := list.Items[0]
	if a.ContextKey != "plan.tier" || a.Type != ContextAttributeTypeString || a.UsageCount == nil || *a.UsageCount != 3 {
		t.Errorf("decoded = %+v", a)
	}
	if want := BasePath + "/projects/checkout/context-attributes?add-fields=usagesCount&page=1&page-size=100"; reqs[0] != want {
		t.Errorf("list request = %q, want %q", reqs[0], want)
	}

	usage, err := h.GetContextAttributeUsage(context.Background(), "checkout", "$key:x")
	if err != nil {
		t.Fatalf("GetContextAttributeUsage: %v", err)
	}
	if len(usage.Items) != 1 || usage.Items[0].ReleaseStages[0].ReleaseStageKey != "production" {
		t.Errorf("usage = %+v", usage)
	}
}
