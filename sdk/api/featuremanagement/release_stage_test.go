package featuremanagement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func TestListReleaseStages_PagesAndEscapesProject(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath()+"?page="+r.URL.Query().Get("page"))
		total := int64(3)
		var items []ReleaseStageDetails
		switch r.URL.Query().Get("page") {
		case "1":
			items = []ReleaseStageDetails{{Key: "dev"}, {Key: "staging"}}
		case "2":
			items = []ReleaseStageDetails{{Key: "production"}}
		}
		_ = json.NewEncoder(w).Encode(ReleaseStageList{Items: items, Pagination: &Pagination{TotalCount: &total}})
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewHandler(c).ListReleaseStages(context.Background(), "a/b", ListOptions{PageSize: 2})
	if err != nil {
		t.Fatalf("ListReleaseStages: %v", err)
	}
	if len(list.Items) != 3 || list.Items[2].Key != "production" {
		t.Fatalf("items = %+v", list.Items)
	}
	want := BasePath + "/projects/a%2Fb/release-stages?page=1"
	if paths[0] != want {
		t.Errorf("first request = %q, want %q", paths[0], want)
	}
}

func TestGetReleaseStage_DecodesAndEscapes(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"key":"production","name":"Production","description":"","type":"PRODUCTION","guarded":true,"displayOrder":2,"modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z","lastModifiedBy":"u","lastModifiedTime":"2026-11-03T10:00:00Z"},"version":3}`))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := NewHandler(c).GetReleaseStage(context.Background(), "checkout", "prod:x")
	if err != nil {
		t.Fatalf("GetReleaseStage: %v", err)
	}
	if rs.Type != ReleaseStageTypeProduction || rs.Guarded == nil || !*rs.Guarded || rs.DisplayOrder != 2 || rs.Version != 3 {
		t.Errorf("decoded = %+v", rs)
	}
	if want := BasePath + "/projects/checkout/release-stages/prod%3Ax"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}
