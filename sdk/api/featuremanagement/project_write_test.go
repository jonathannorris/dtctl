package featuremanagement

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// writeCall is one request a recordWrites server received.
type writeCall struct{ method, path, version, body string }

// recordWrites serves respond's body (204 for a delete) and records every
// request.
func recordWrites(t *testing.T, respond string) (*Handler, *[]writeCall) {
	t.Helper()
	var calls []writeCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, writeCall{r.Method, strings.TrimPrefix(r.URL.EscapedPath(), BasePath), r.URL.Query().Get("optimistic-locking-version"), string(b)})
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(respond))
	}))
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(c), &calls
}

const projectWithUnknownJSON = `{"key":"checkout","name":"Checkout","description":"d","owner":"team","maintainers":["a"],
 "settings":{"changeRequests":{"enabled":true,"minApprovals":2},"retention":"P30D"},"labels":["web"],"version":7,
 "modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z","lastModifiedBy":"u","lastModifiedTime":"2026-11-03T10:00:00Z"}}`

func TestNewProjectUpdate_CarriesUnknownMembers(t *testing.T) {
	var d ProjectDetails
	if err := json.Unmarshal([]byte(projectWithUnknownJSON), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(NewProjectUpdate(&d))
	s := string(out)
	for _, want := range []string{`"labels":["web"]`, `"retention":"P30D"`, `"minApprovals":2`, `"enabled":true`} {
		if !strings.Contains(s, want) {
			t.Errorf("update lacks %s: %s", want, s)
		}
	}
	for _, readOnly := range []string{`"key"`, `"version"`, `"modificationInfo"`} {
		if strings.Contains(s, readOnly) {
			t.Errorf("update sends read-only %s: %s", readOnly, s)
		}
	}
}

func TestProjectWrites_MethodsPathsAndVersions(t *testing.T) {
	h, calls := recordWrites(t, projectWithUnknownJSON)
	ctx := context.Background()
	if p, err := h.CreateProject(ctx, ProjectCreate{Key: "a/b", Name: "A"}); err != nil || p.Key != "checkout" {
		t.Fatalf("create: %+v, %v", p, err)
	}
	if _, err := h.UpdateProject(ctx, "a/b", ProjectUpdate{Name: "A"}, "7"); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteProject(ctx, "a/b"); err != nil {
		t.Fatal(err)
	}
	want := []writeCall{
		{"POST", "/projects", "", `{"key":"a/b","name":"A"}`},
		{"PUT", "/projects/a%2Fb", "7", ""},
		{"DELETE", "/projects/a%2Fb", "", ""},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || got.version != w.version || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
}
