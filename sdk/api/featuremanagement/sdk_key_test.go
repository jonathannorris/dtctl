package featuremanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

func TestSDKKeyPublicID(t *testing.T) {
	tests := map[string]string{
		"dt01.server_us_Abcdefghij.0123456789abcdef0123456789abcdef_0123456789": "dt01.server_us_Abcdefghij",
		"dt01.server_us_Abcdefghij": "",
		"no-dots":                   "",
		"a..b":                      "",
		"":                          "",
	}
	for in, want := range tests {
		if got := SDKKeyPublicID(in); got != want {
			t.Errorf("SDKKeyPublicID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskSDKKey(t *testing.T) {
	tests := map[string]string{
		"dt01.server_us_Abcdefghij.0123456789abcdef0123456789abcdef_0123456789": "dt01.server_us_Abcdefghij.********",
		"dt01.server_us_Abcdefghij": "",
		"":                          "",
	}
	for in, want := range tests {
		if got := MaskSDKKey(in); got != want {
			t.Errorf("MaskSDKKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListSDKKeys(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"web":[],"mobile":[],"server":[{"sdkKey":"dt01.server_us_Abcdefghij.0123456789abcdef0123456789abcdef_0123456789","type":"server","version":1}]}`))
	}))
	defer srv.Close()

	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewHandler(c).ListSDKKeys(context.Background(), "checkout", "prod:x")
	if err != nil {
		t.Fatalf("ListSDKKeys: %v", err)
	}
	if want := BasePath + "/projects/checkout/release-stages/prod%3Ax/sdk-keys"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if len(list.Server) != 1 || list.Server[0].Type != SDKKeyTypeServer || len(list.Web) != 0 {
		t.Errorf("list = %+v", list)
	}
}
