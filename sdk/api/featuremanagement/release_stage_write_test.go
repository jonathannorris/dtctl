package featuremanagement

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const releaseStageWithUnknownJSON = `{"key":"prod","name":"Production","description":"","type":"PRODUCTION","guarded":true,
 "displayOrder":2,"color":"red","version":5,"modificationInfo":{"createdBy":"u","createdTime":"2026-11-03T10:00:00Z"}}`

func TestNewReleaseStageUpdate_CarriesUnknownMembers(t *testing.T) {
	var d ReleaseStageDetails
	if err := json.Unmarshal([]byte(releaseStageWithUnknownJSON), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(NewReleaseStageUpdate(&d))
	if s := string(out); !strings.Contains(s, `"color":"red"`) || !strings.Contains(s, `"guarded":true`) ||
		!strings.Contains(s, `"displayOrder":2`) || strings.Contains(s, `"key"`) || strings.Contains(s, `"version"`) {
		t.Errorf("update = %s", s)
	}
}

func TestReleaseStageWrites_MethodsPathsAndVersions(t *testing.T) {
	h, calls := recordWrites(t, releaseStageWithUnknownJSON)
	ctx := context.Background()
	if _, err := h.CreateReleaseStage(ctx, "checkout", ReleaseStageCreate{Key: "qa", Name: "QA", Type: "STAGING"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.UpdateReleaseStage(ctx, "checkout", "q:a", ReleaseStageUpdate{Name: "QA", Type: "STAGING"}, "5"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ReorderReleaseStages(ctx, "checkout", ReleaseStageReorder{Items: []ReleaseStageOrder{{Key: "qa", DisplayOrder: 0, Version: 5}}}); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteReleaseStage(ctx, "checkout", "q:a"); err != nil {
		t.Fatal(err)
	}
	want := []writeCall{
		{"POST", "/projects/checkout/release-stages", "", `{"key":"qa","name":"QA","type":"STAGING"}`},
		{"PUT", "/projects/checkout/release-stages/q%3Aa", "5", ""},
		{"PUT", "/projects/checkout/release-stages/reorder", "", `{"items":[{"key":"qa","displayOrder":0,"version":5}]}`},
		{"DELETE", "/projects/checkout/release-stages/q%3Aa", "", ""},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || got.version != w.version || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
}
