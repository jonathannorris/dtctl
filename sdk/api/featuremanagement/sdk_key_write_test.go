package featuremanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

const testSDKKey = "dt01.server_us_AbCd123456.0123456789abcdef0123456789abcdef_0123456789"

func TestSDKKeyWrites_MethodsAndPaths(t *testing.T) {
	h, calls := recordWrites(t, `{"sdkKey":"`+testSDKKey+`","type":"server"}`)
	ctx := context.Background()
	k, err := h.CreateSDKKey(ctx, "checkout", "production", SDKKeyCreate{Type: SDKKeyTypeServer})
	if err != nil || k.SDKKey != testSDKKey {
		t.Fatalf("create: %+v, %v", k, err)
	}
	if err := h.DeleteSDKKey(ctx, "checkout", "production", testSDKKey); err != nil {
		t.Fatal(err)
	}
	want := []writeCall{
		{"POST", "/projects/checkout/release-stages/production/sdk-keys", "", `{"type":"server"}`},
		{"DELETE", "/projects/checkout/release-stages/production/sdk-keys/" + testSDKKey, "", ""},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || (w.body != "" && got.body != w.body) {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestDeleteSDKKey_ErrorKeepsTheSecretOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"SDK key not found"}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	err = NewHandler(c).DeleteSDKKey(context.Background(), "checkout", "production", testSDKKey)
	if err == nil || strings.Contains(err.Error(), "0123456789abcdef") || !strings.Contains(err.Error(), "dt01.server_us_AbCd123456") {
		t.Errorf("err = %v", err)
	}
}
