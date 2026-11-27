package featuremanagement

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const contextAttributeWithUnknownJSON = `{"projectKey":"checkout","key":"country","name":"Country","description":"",
 "schema":"","type":"STRING","contextKey":"user","builtIn":false,"usageCount":3,"examples":["AT"],"version":2}`

func TestNewContextAttributeUpdate_SendsOnlyWhatAnUpdateTakes(t *testing.T) {
	var d ContextAttributeDetails
	if err := json.Unmarshal([]byte(contextAttributeWithUnknownJSON), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(NewContextAttributeUpdate(&d))
	s := string(out)
	if !strings.Contains(s, `"examples":["AT"]`) || !strings.Contains(s, `"schema":""`) {
		t.Errorf("update = %s", s)
	}
	for _, readOnly := range []string{"projectKey", "key", "type", "contextKey", "builtIn", "usageCount", "version"} {
		if strings.Contains(s, `"`+readOnly+`"`) {
			t.Errorf("update sends read-only %s: %s", readOnly, s)
		}
	}
}

func TestContextAttributeWrites_MethodsPathsAndVersions(t *testing.T) {
	h, calls := recordWrites(t, contextAttributeWithUnknownJSON)
	ctx := context.Background()
	if _, err := h.CreateContextAttribute(ctx, "checkout", ContextAttributeCreate{Key: "plan", Name: "Plan", Type: "STRING", ContextKey: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.UpdateContextAttribute(ctx, "checkout", "plan", ContextAttributeUpdate{Name: "Plan"}, "2"); err != nil {
		t.Fatal(err)
	}
	if err := h.DeleteContextAttribute(ctx, "checkout", "plan"); err != nil {
		t.Fatal(err)
	}
	want := []writeCall{
		{"POST", "/projects/checkout/context-attributes", "", `{"key":"plan","name":"Plan","schema":"","type":"STRING","contextKey":"user"}`},
		{"PUT", "/projects/checkout/context-attributes/plan", "2", `{"name":"Plan","description":"","schema":""}`},
		{"DELETE", "/projects/checkout/context-attributes/plan", "", ""},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || got.version != w.version || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
}
