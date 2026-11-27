package featuremanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func newTestHandler(t *testing.T, handler http.Handler) *Handler {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return NewHandler(c)
}

func testProject(key string) ProjectDetails {
	enabled := true
	ts := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	return ProjectDetails{
		Key:         key,
		Name:        "Project " + key,
		Description: "Synthetic project " + key,
		Owner:       "team-" + key,
		Maintainers: []string{"maintainer-a"},
		Settings:    ProjectSettings{ChangeRequests: &ChangeRequestSettings{Enabled: &enabled}},
		ModificationInfo: ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-000000000001", CreatedTime: ts,
			LastModifiedBy: "00000000-0000-4000-8000-000000000002", LastModifiedTime: ts,
		},
		Version: 3,
	}
}

// pagedProjects serves n synthetic projects with the API's page-number
// pagination, and records every request's query.
func pagedProjects(t *testing.T, n int, withTotal bool, queries *[]string) http.Handler {
	t.Helper()
	all := make([]ProjectDetails, n)
	for i := range all {
		all[i] = testProject(fmt.Sprintf("project-%03d", i))
	}
	mux := http.NewServeMux()
	mux.HandleFunc(BasePath+"/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		*queries = append(*queries, r.URL.RawQuery)
		if q.Get("page-key") != "" {
			// Feature Management has no page keys; a client sending one is
			// following the wrong pagination model.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		page, _ := strconv.Atoi(q.Get("page"))
		size, _ := strconv.Atoi(q.Get("page-size"))
		if page < 1 || size < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		start := min((page-1)*size, n)
		end := min(start+size, n)
		resp := ProjectList{Items: all[start:end], Pagination: &Pagination{Page: &page, PageSize: &size}}
		if withTotal {
			total := int64(n)
			resp.Pagination.TotalCount = &total
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

func TestListProjects_FollowsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		n         int
		withTotal bool
		wantCalls int
	}{
		{"total count, partial last page", 5, true, 3},
		{"total count, exact multiple", 4, true, 2},
		{"no total count, partial last page", 5, false, 3},
		// Without a total, a full last page needs one empty page to prove it
		// was the last.
		{"no total count, exact multiple", 4, false, 3},
		{"empty", 0, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			h := newTestHandler(t, pagedProjects(t, tc.n, tc.withTotal, &queries))

			list, err := h.ListProjects(context.Background(), ListOptions{PageSize: 2})
			if err != nil {
				t.Fatalf("ListProjects: %v", err)
			}
			if len(list.Items) != tc.n {
				t.Errorf("got %d projects, want %d", len(list.Items), tc.n)
			}
			for i, p := range list.Items {
				if want := fmt.Sprintf("project-%03d", i); p.Key != want {
					t.Errorf("item %d key = %q, want %q", i, p.Key, want)
				}
			}
			if len(queries) != tc.wantCalls {
				t.Errorf("made %d requests (%v), want %d", len(queries), queries, tc.wantCalls)
			}
			if list.Items == nil {
				t.Error("Items is nil; an empty listing must encode as [] rather than null")
			}
			if list.Pagination != nil {
				t.Error("merged listing kept a single page's pagination")
			}
		})
	}
}

func TestListProjects_DefaultPageSizeAndOptions(t *testing.T) {
	var got []string
	mux := http.NewServeMux()
	mux.HandleFunc(BasePath+"/projects", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		got = append(got, q.Get("page-size"), q.Get("sort"), strings.Join(q["add-fields"], ","))
		_, _ = w.Write([]byte(`{"items":[],"resourceContext":{"operations":["write"]}}`))
	})
	h := newTestHandler(t, mux)

	list, err := h.ListProjects(context.Background(), ListOptions{Sort: "-lastModifiedAt", AddFields: []string{"resourceContext"}})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	want := []string{strconv.Itoa(DefaultPageSize), "-lastModifiedAt", "resourceContext"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("query = %v, want %v", got, want)
	}
	if list.ResourceContext == nil || len(list.ResourceContext.Operations) != 1 {
		t.Errorf("collection resourceContext not kept: %+v", list.ResourceContext)
	}
}

func TestListProjects_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(BasePath+"/projects", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"missing scope feature-management:projects:read"}}`))
	})
	h := newTestHandler(t, mux)

	_, err := h.ListProjects(context.Background(), ListOptions{})
	if err == nil || !strings.Contains(err.Error(), "missing scope feature-management:projects:read") {
		t.Fatalf("error = %v, want the nested error.message", err)
	}
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("error is not a 403 APIError: %v", err)
	}
}

func TestGetProject(t *testing.T) {
	want := testProject("checkout")
	mux := http.NewServeMux()
	mux.HandleFunc(BasePath+"/projects/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if got := r.URL.Query()["add-fields"]; len(got) != 1 || got[0] != "resourceContext" {
			t.Errorf("add-fields = %v", got)
		}
		_ = json.NewEncoder(w).Encode(want)
	})
	h := newTestHandler(t, mux)

	got, err := h.GetProject(context.Background(), "checkout", "resourceContext")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Key != want.Key || got.Version != want.Version || got.Owner != want.Owner {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Settings.ChangeRequests == nil || got.Settings.ChangeRequests.Enabled == nil || !*got.Settings.ChangeRequests.Enabled {
		t.Errorf("settings.changeRequests.enabled not decoded: %+v", got.Settings)
	}
	if !got.ModificationInfo.LastModifiedTime.Equal(want.ModificationInfo.LastModifiedTime) {
		t.Errorf("modificationInfo not decoded: %+v", got.ModificationInfo)
	}
}

// A key reaches the server as one path segment, whatever it contains.
func TestGetProject_EscapesKey(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Project not found"}}`))
	})
	h := newTestHandler(t, mux)

	_, err := h.GetProject(context.Background(), "a/b:c#d")
	if err == nil || !strings.Contains(err.Error(), "Project not found") {
		t.Fatalf("error = %v", err)
	}
	if want := BasePath + "/projects/a%2Fb%3Ac%23d"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
