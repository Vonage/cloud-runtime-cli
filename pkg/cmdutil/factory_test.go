package cmdutil

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/config"
)

// recordingServer is an httptest server that keeps the headers of every request it receives.
type recordingServer struct {
	*httptest.Server

	mu      sync.Mutex
	headers []http.Header
}

func newRecordingServer(t *testing.T, handler http.HandlerFunc) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.headers = append(rs.headers, r.Header.Clone())
		rs.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) requests() []http.Header {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]http.Header(nil), rs.headers...)
}

func newFakeGitHub(t *testing.T) *recordingServer {
	t.Helper()
	return newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","assets":[]}`))
			return
		}
		_, _ = w.Write([]byte("asset-bytes"))
	})
}

func newFakePlatform(t *testing.T) *recordingServer {
	t.Helper()
	return newRecordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"Regions":[{"alias":"aws.euw1","deployment_api_url":"https://deploy.example.com"}]}}`))
	})
}

// credentialHeaders returns the names of the headers that carry platform credentials.
func credentialHeaders(h http.Header) []string {
	var names []string
	for name := range h {
		if name == "Authorization" || strings.HasPrefix(name, "X-Neru-") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func TestReleaseClientSendsNoPlatformCredentials(t *testing.T) {
	const apiKey, apiSecret = "abcd1234", "Te5ts3cret"

	tests := []struct {
		name string
		init func(ctx context.Context, f *DefaultFactory, platformURL string) error
	}{
		{
			name: "update-check-after-init",
			init: func(ctx context.Context, f *DefaultFactory, platformURL string) error {
				cfg := config.CLIConfig{GraphqlEndpoint: platformURL, DefaultRegion: "aws.euw1", Credentials: config.Credentials{APIKey: apiKey, APISecret: apiSecret}}
				return f.Init(ctx, cfg, &config.GlobalOptions{Timeout: time.Minute})
			},
		},
		{
			name: "upgrade-after-init-upgrade",
			init: func(_ context.Context, f *DefaultFactory, _ string) error {
				f.InitUpgrade(&config.GlobalOptions{Timeout: time.Minute})
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			github := newFakeGitHub(t)
			platform := newFakePlatform(t)
			ctx := t.Context()

			f := NewDefaultFactory("v0.4", github.URL)
			require.NoError(t, tt.init(ctx, f, platform.URL))

			release, err := f.ReleaseClient().GetLatestRelease(ctx)
			require.NoError(t, err)
			require.Equal(t, "v9.9.9", release.TagName)
			asset, err := f.ReleaseClient().GetAsset(ctx, github.URL+"/download/vcr.tar.gz")
			require.NoError(t, err)
			require.Equal(t, "asset-bytes", string(asset))

			requests := github.requests()
			require.Len(t, requests, 2)
			for _, h := range requests {
				require.Empty(t, credentialHeaders(h), "release host must not receive platform credentials")
			}
		})
	}
}

func TestInitKeepsPlatformCredentialsOnPlatformClient(t *testing.T) {
	const apiKey, apiSecret = "abcd1234", "Te5ts3cret"
	platform := newFakePlatform(t)

	f := NewDefaultFactory("v0.4", "https://github.example.com")
	cfg := config.CLIConfig{GraphqlEndpoint: platform.URL, DefaultRegion: "aws.euw1", Credentials: config.Credentials{APIKey: apiKey, APISecret: apiSecret}}
	require.NoError(t, f.Init(t.Context(), cfg, &config.GlobalOptions{Timeout: time.Minute}))

	requests := platform.requests()
	require.Len(t, requests, 1)
	require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte(apiKey+":"+apiSecret)), requests[0].Get("Authorization"))
	require.Equal(t, apiKey, requests[0].Get("X-Neru-ApiAccountId"))
	require.NotEmpty(t, requests[0].Get("X-Neru-TraceId"))
}
