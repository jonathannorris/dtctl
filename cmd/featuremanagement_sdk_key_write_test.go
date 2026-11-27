package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

const (
	fmTestSDKKey    = "dt01.server_us_AbCd123456.0123456789abcdef0123456789abcdef_0123456789"
	fmTestSDKSecret = "0123456789abcdef0123456789abcdef"
)

// newFMSDKKeyServer serves the production stage with one server key and
// records SDK key writes.
func newFMSDKKeyServer(t *testing.T) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	stage := sdkfm.BasePath + "/projects/checkout/release-stages/production"
	mux := http.NewServeMux()
	mux.HandleFunc(stage, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"production","name":"Production","type":"PRODUCTION","version":1}`))
	})
	mux.HandleFunc(stage+"/sdk-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.puts = append(s.puts, map[string]any{"_method": "POST", "body": string(b)})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sdkKey":"dt01.server_us_ZzZz999999.ffffffffffffffffffffffffffffffff_0123456789","type":"server"}`))
			return
		}
		_, _ = w.Write([]byte(`{"server":[{"sdkKey":"` + fmTestSDKKey + `","type":"server"}],"web":[],"mobile":[]}`))
	})
	mux.HandleFunc(stage+"/sdk-keys/", func(w http.ResponseWriter, r *http.Request) {
		s.puts = append(s.puts, map[string]any{"_method": r.Method, "_path": r.URL.Path})
		w.WriteHeader(http.StatusNoContent)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestFMCreateSDKKey_PrintsTheKeyAndWarnsAboutExistingOnes(t *testing.T) {
	srv := newFMSDKKeyServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newCreateFMCmd(), "sdk-key", "production", "--type", "server")
	require.Error(t, err)
	require.Empty(t, srv.puts)

	out, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newCreateFMCmd(), "sdk-key", "production", "--type", "server")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	require.Equal(t, `{"type":"server"}`, srv.puts[0]["body"])
	require.Contains(t, out, "dt01.server_us_ZzZz999999.ffffffffffffffffffffffffffffffff_0123456789")
}

func TestFMDeleteSDKKey_AcceptsTheMaskedForm(t *testing.T) {
	for _, arg := range []string{"dt01.server_us_AbCd123456.********", "dt01.server_us_AbCd123456", fmTestSDKKey} {
		srv := newFMSDKKeyServer(t)
		_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "sdk-key", "production", arg, "-y")
		require.Error(t, err, "readwrite-all must refuse an SDK key delete")
		require.Empty(t, srv.puts)

		out, err := runFMWrite(t, srv, config.SafetyLevelDangerouslyUnrestricted, newDeleteFMCmd(), "sdk-key", "production", arg, "-y")
		require.NoError(t, err, arg)
		require.Len(t, srv.puts, 1)
		require.True(t, strings.HasSuffix(srv.puts[0]["_path"].(string), "/sdk-keys/"+fmTestSDKKey), "%v", srv.puts[0])
		require.NotContains(t, out, fmTestSDKSecret, "the result must not echo the secret")
	}
}

func TestFMDeleteSDKKey_UnknownKeyNeverEchoesTheSecret(t *testing.T) {
	srv := newFMSDKKeyServer(t)
	other := "dt01.server_us_Nope000000." + fmTestSDKSecret + "_0123456789"
	_, err := runFMWrite(t, srv, config.SafetyLevelDangerouslyUnrestricted, newDeleteFMCmd(), "sdk-key", "production", other, "-y")
	require.ErrorContains(t, err, "has no SDK key dt01.server_us_Nope000000.********")
	require.NotContains(t, err.Error(), fmTestSDKSecret)
	require.Empty(t, srv.puts)
}
