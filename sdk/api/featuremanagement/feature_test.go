package featuremanagement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func TestListFeatures_Pages(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath()+"?page="+r.URL.Query().Get("page"))
		total := int64(2)
		var items []FeatureDetails
		switch r.URL.Query().Get("page") {
		case "1":
			items = []FeatureDetails{{Key: "a"}}
		case "2":
			items = []FeatureDetails{{Key: "b"}}
		}
		_ = json.NewEncoder(w).Encode(FeatureList{Items: items, Pagination: &Pagination{TotalCount: &total}})
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewHandler(c).ListFeatures(context.Background(), "checkout", ListOptions{PageSize: 1})
	if err != nil {
		t.Fatalf("ListFeatures: %v", err)
	}
	if len(list.Items) != 2 || list.Items[1].Key != "b" {
		t.Fatalf("items = %+v", list.Items)
	}
	if want := BasePath + "/projects/checkout/features?page=1"; paths[0] != want {
		t.Errorf("first request = %q, want %q", paths[0], want)
	}
}

func TestGetFeature_DecodesFlags(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"projectKey":"checkout","key":"new-checkout","name":"New checkout","description":"",
			"variants":[{"key":"off","name":"Off","displayOrder":0},{"key":"on","name":"On","displayOrder":1}],
			"flags":[
			  {"key":"enabled","name":"Enabled","description":"","type":"BOOLEAN","variants":{"off":false,"on":true}},
			  {"key":"color","name":"Color","description":"","type":"STRING","variants":{"off":"blue","on":"green"},
			   "schema":{"type":"ENUM","values":["blue","green"]}}],
			"owner":"o","maintainers":[],"status":"ACTIVE","type":"RELEASE","version":2}`))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewHandler(c).GetFeature(context.Background(), "checkout", "a:b")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if want := BasePath + "/projects/checkout/features/a%3Ab"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if len(f.Variants) != 2 || len(f.Flags) != 2 || f.Status != FeatureStatusActive || f.Type != FeatureTypeRelease {
		t.Fatalf("decoded = %+v", f)
	}
	if f.Flags[0].Variants["on"] != true || f.Flags[1].Variants["on"] != "green" {
		t.Errorf("flag variants = %+v", f.Flags)
	}
	if f.Flags[1].Schema["type"] != "ENUM" {
		t.Errorf("flag schema = %+v", f.Flags[1].Schema)
	}
}
