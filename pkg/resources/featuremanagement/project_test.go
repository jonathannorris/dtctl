package featuremanagement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Dynatrace/dtctl/pkg/client"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

const projectsPath = sdkfm.BasePath + "/projects"

// keyPattern is the API's project key pattern.
var keyPattern = regexp.MustCompile(`^[a-z0-9][-a-z0-9_.]{0,254}[a-z0-9]$`)

func sdkProject(key, name string, changeRequests bool) sdkfm.ProjectDetails {
	ts := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	return sdkfm.ProjectDetails{
		Key:         key,
		Name:        name,
		Description: "Synthetic project",
		Owner:       "team-a",
		Settings:    sdkfm.ProjectSettings{ChangeRequests: &sdkfm.ChangeRequestSettings{Enabled: &changeRequests}},
		ModificationInfo: sdkfm.ModificationInfo{
			CreatedBy: "00000000-0000-4000-8000-000000000001", CreatedTime: ts,
			LastModifiedBy: "00000000-0000-4000-8000-000000000001", LastModifiedTime: ts,
		},
		Version: 1,
	}
}

// fakeFM serves a fixed set of projects: the list on /projects, and each
// project by key, with the API's 404 envelope for an unknown key.
func fakeFM(t *testing.T, projects ...sdkfm.ProjectDetails) (*Handler, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc(projectsPath, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "list")
		_ = json.NewEncoder(w).Encode(sdkfm.ProjectList{Items: projects})
	})
	mux.HandleFunc(projectsPath+"/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, projectsPath+"/")
		calls = append(calls, "get "+key)
		// The API validates the key pattern before looking anything up.
		if !keyPattern.MatchString(key) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
			return
		}
		for _, p := range projects {
			if p.Key == key {
				_ = json.NewEncoder(w).Encode(p)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Project not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("client.NewForTesting: %v", err)
	}
	return NewHandler(c), &calls
}

func TestListProjects_SortsAndFillsDisplayFields(t *testing.T) {
	h, _ := fakeFM(t,
		sdkProject("payments", "Payments", false),
		sdkProject("checkout", "Checkout", true),
	)

	projects, err := h.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 || projects[0].Key != "checkout" || projects[1].Key != "payments" {
		t.Fatalf("projects = %+v, want checkout then payments", projects)
	}
	if projects[0].ChangeRequestsDisplay != "on" || projects[1].ChangeRequestsDisplay != "off" {
		t.Errorf("change requests display = %q, %q", projects[0].ChangeRequestsDisplay, projects[1].ChangeRequestsDisplay)
	}
	if projects[0].LastModifiedDisplay != "2026-11-03 10:00:00" {
		t.Errorf("last modified display = %q", projects[0].LastModifiedDisplay)
	}
	if projects[0].Maintainers == nil {
		t.Error("absent maintainers must encode as [] rather than null")
	}
}

func TestGetProject_KeyFirst(t *testing.T) {
	h, calls := fakeFM(t, sdkProject("checkout", "Checkout", false))

	p, err := h.GetProject("checkout")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Key != "checkout" {
		t.Errorf("key = %q", p.Key)
	}
	if strings.Join(*calls, ",") != "get checkout" {
		t.Errorf("calls = %v, want one GET by key and no listing", *calls)
	}
}

func TestGetProject_FallsBackToName(t *testing.T) {
	h, calls := fakeFM(t,
		sdkProject("checkout", "Checkout flows", false),
		sdkProject("payments", "Payments", false),
	)

	p, err := h.GetProject("Checkout flows")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Key != "checkout" {
		t.Errorf("key = %q, want checkout", p.Key)
	}
	if strings.Join(*calls, ",") != "get Checkout flows,list" {
		t.Errorf("calls = %v, want the key first, then a listing", *calls)
	}
}

// A name that is also a well-formed key misses with 404, not 400, and must
// still resolve by name.
func TestGetProject_FallsBackToKeyShapedName(t *testing.T) {
	h, _ := fakeFM(t, sdkProject("checkout", "legacy-checkout", false))

	p, err := h.GetProject("legacy-checkout")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Key != "checkout" {
		t.Errorf("key = %q, want checkout", p.Key)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	h, _ := fakeFM(t, sdkProject("checkout", "Checkout", false))

	_, err := h.GetProject("nope")
	if err == nil || !strings.Contains(err.Error(), `project "nope" not found`) {
		t.Fatalf("error = %v", err)
	}
}

func TestGetProject_AmbiguousName(t *testing.T) {
	h, _ := fakeFM(t,
		sdkProject("shop-b", "Shop", false),
		sdkProject("shop-a", "Shop", false),
	)

	_, err := h.GetProject("Shop")
	if err == nil || !strings.Contains(err.Error(), "2 projects are named \"Shop\"; pass a key instead: shop-a, shop-b") {
		t.Fatalf("error = %v", err)
	}
}

// A failure other than 404 is the answer; it must not trigger a name lookup
// that could mask it.
func TestGetProject_OtherErrorsDoNotFallBack(t *testing.T) {
	var listed bool
	mux := http.NewServeMux()
	mux.HandleFunc(projectsPath, func(w http.ResponseWriter, r *http.Request) { listed = true })
	mux.HandleFunc(projectsPath+"/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Forbidden"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewHandler(c).GetProject("checkout")
	if err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("error = %v", err)
	}
	if listed {
		t.Error("a 403 fell back to a name lookup")
	}
}
