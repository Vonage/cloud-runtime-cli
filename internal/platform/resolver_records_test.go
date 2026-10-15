package platform

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegionRecordURLChecks(t *testing.T) {
	const (
		insecure     = "the platform returned an insecure URL for "
		noDeployment = "the platform returned a region without a deploymentApiUrl"
	)
	tests := []struct {
		name    string
		edit    func(r *Region)
		wantErr string
	}{
		{name: "plain http deployment", edit: func(r *Region) { r.DeploymentAPIURL = "http://example.test/api" },
			wantErr: insecure + "deploymentApiUrl: http://example.test/api"},
		{name: "plain http marketplace", edit: func(r *Region) { r.MarketplaceAPIURL = "http://example.test" },
			wantErr: insecure + "marketplaceApiUrl: http://example.test"},
		{name: "other scheme for assets", edit: func(r *Region) { r.AssetsAPIURL = "ftp://example.test" },
			wantErr: insecure + "assetsApiUrl: ftp://example.test"},
		{name: "websocket scheme", edit: func(r *Region) { r.DeploymentAPIURL = "wss://example.test/api" },
			wantErr: insecure + "deploymentApiUrl: wss://example.test/api"},
		{name: "userinfo", edit: func(r *Region) { r.DeploymentAPIURL = "https://user:pw@example.test/" },
			wantErr: insecure + "deploymentApiUrl: https://user:pw@example.test/"},
		{name: "empty userinfo", edit: func(r *Region) { r.MarketplaceAPIURL = "https://@example.test/" },
			wantErr: insecure + "marketplaceApiUrl: https://@example.test/"},
		{name: "no host", edit: func(r *Region) { r.DeploymentAPIURL = "https:///api" },
			wantErr: insecure + "deploymentApiUrl: https:///api"},
		{name: "opaque", edit: func(r *Region) { r.AssetsAPIURL = "https:example.test" },
			wantErr: insecure + "assetsApiUrl: https:example.test"},
		{name: "unparsable", edit: func(r *Region) { r.DeploymentAPIURL = "https://exa mple.test" },
			wantErr: insecure + "deploymentApiUrl: https://exa mple.test"},
		{name: "no scheme", edit: func(r *Region) { r.MarketplaceAPIURL = "example.test/mp" },
			wantErr: insecure + "marketplaceApiUrl: example.test/mp"},
		{name: "empty deployment", edit: func(r *Region) { r.DeploymentAPIURL = "" }, wantErr: noDeployment},
		{name: "escape sequence is dropped from the message", edit: func(r *Region) { r.DeploymentAPIURL = "https://example.test/\x1b[31mred" },
			wantErr: insecure + "deploymentApiUrl: https://example.test/[31mred"},
		{name: "empty assets and marketplace are accepted", edit: func(r *Region) {
			r.DeploymentAPIURL = "https://example.test/api"
			r.AssetsAPIURL, r.MarketplaceAPIURL = "", ""
		}},
		{name: "https everywhere", edit: func(r *Region) {
			r.DeploymentAPIURL = "https://example.test/api"
			r.AssetsAPIURL = "https://assets.example.test"
			r.MarketplaceAPIURL = "https://example.test:8443/mp/"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := recordAt("https://example.test")
			tt.edit(&rec)
			srv := recordServer(t, rec)
			r := newTestResolver(t, srv.URL, "v0.3", nil)

			got, err := r.Region(t.Context(), "aws.euw1")
			client, clientErr := r.Client(t.Context(), "aws.euw1")

			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NoError(t, clientErr)
				assert.Equal(t, rec, got)
				assert.Equal(t, rec, client.Region())
				return
			}
			require.EqualError(t, err, tt.wantErr)
			require.EqualError(t, clientErr, tt.wantErr)
			assert.Nil(t, client)
			assert.NotContains(t, err.Error(), "\x1b")
		})
	}
}

func TestRegionRecordURLInTheErrorIsCapped(t *testing.T) {
	const prefix = "the platform returned an insecure URL for deploymentApiUrl: "
	rec := recordAt("https://example.test")
	rec.DeploymentAPIURL = "http://example.test/x" + strings.Repeat("é", 300)
	srv := recordServer(t, rec)
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	_, err := r.Region(t.Context(), "aws.euw1")

	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), prefix), err.Error())
	shown := strings.TrimPrefix(err.Error(), prefix)
	assert.Len(t, shown, 199, "the cut stops before the character that would pass 200 bytes")
	assert.True(t, strings.HasPrefix(rec.DeploymentAPIURL, shown))
}

func TestRejectedRecordAddsNoHostToThePlatformSet(t *testing.T) {
	deployment := newRecorder(nil)
	srvDeployment := startPlain(t, deployment)
	rec := recordAt(srvDeployment.URL)
	rec.MarketplaceAPIURL = "http://203.0.113.7:8080/mp"
	srv := recordServer(t, rec)
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	_, err := r.Region(t.Context(), "aws.euw1")

	require.EqualError(t, err, "the platform returned an insecure URL for marketplaceApiUrl: http://203.0.113.7:8080/mp")
	assert.False(t, r.core.isPlatformHost(mustURL(t, srvDeployment.URL)))
	assert.False(t, r.core.isPlatformHost(mustURL(t, "http://203.0.113.7:8080")))
	assert.True(t, r.core.isPlatformHost(mustURL(t, srv.URL)), "the bootstrap host stays")

	rep, err := r.core.send(t.Context(), http.MethodGet, mustURL(t, srvDeployment.URL+"/x"), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())
	require.Equal(t, 1, deployment.count())
	assertNoCredentialHeaders(t, deployment.requests()[0].Header)
}

func TestLoopbackBootstrapAcceptsLoopbackRecordOnly(t *testing.T) {
	t.Run("a loopback http record is accepted", func(t *testing.T) {
		other := startPlain(t, newRecorder(nil))
		rec := recordAt(other.URL)
		srv := recordServer(t, rec)
		r := newTestResolver(t, srv.URL, "v0.3", nil)

		got, err := r.Region(t.Context(), "aws.euw1")

		require.NoError(t, err)
		assert.Equal(t, other.URL+"/dep", got.DeploymentAPIURL)
	})

	t.Run("a non-loopback http record is refused", func(t *testing.T) {
		rec := recordAt("https://example.test")
		rec.DeploymentAPIURL = "http://203.0.113.7/"
		srv := recordServer(t, rec)
		r := newTestResolver(t, srv.URL, "v0.3", nil)

		_, err := r.Region(t.Context(), "aws.euw1")

		require.EqualError(t, err, "the platform returned an insecure URL for deploymentApiUrl: http://203.0.113.7/")
	})
}

func TestHTTPSBootstrapRefusesLoopbackHTTPRecord(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(r *Region)
		wantErr string
	}{
		{name: "loopback http deployment", edit: func(r *Region) { r.DeploymentAPIURL = "http://127.0.0.1:9/dep" },
			wantErr: "the platform returned an insecure URL for deploymentApiUrl: http://127.0.0.1:9/dep"},
		{name: "localhost http marketplace", edit: func(r *Region) { r.MarketplaceAPIURL = "http://localhost:9/mp" },
			wantErr: "the platform returned an insecure URL for marketplaceApiUrl: http://localhost:9/mp"},
		{name: "https record", edit: func(r *Region) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, r *http.Request) {
				rec := selfRecord(r)
				tt.edit(&rec)
				answerJSON(http.StatusOK, mustJSON(rec))(w, r)
			}))
			r := newTestResolver(t, srv.URL, "v0.3", tlsCfg)

			_, err := r.Region(t.Context(), "aws.euw1")

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestClientSendsCredentialsToRecordHostOnly(t *testing.T) {
	deployment := newRecorder(answerJSON(http.StatusOK, `{"runtimes":[]}`))
	srvDeployment := startPlain(t, deployment)
	unrelated := newRecorder(nil)
	srvUnrelated := startPlain(t, unrelated)
	srv := recordServer(t, recordAt(srvDeployment.URL))
	r := newTestResolver(t, srv.URL, "v0.3", nil)
	c := clientOf(t, r, "aws.euw1")

	_, err := c.ListRuntimes(t.Context())
	require.NoError(t, err)

	require.Equal(t, 1, deployment.count())
	got := deployment.requests()[0]
	assert.Equal(t, "/dep/v0.3/runtimes", got.RawPath)
	assert.NotEmpty(t, got.Header.Get("Authorization"))
	assert.Equal(t, "abc123", got.Header.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, got.Header.Get("X-Neru-Traceid"))

	rep, err := r.core.send(t.Context(), http.MethodGet, mustURL(t, srvUnrelated.URL+"/x"), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())
	require.Equal(t, 1, unrelated.count())
	assertNoCredentialHeaders(t, unrelated.requests()[0].Header)
}

func TestDeploymentAndMarketplaceHostsJoinThePlatformSetButAssetsDoNot(t *testing.T) {
	srvDeployment := startPlain(t, newRecorder(nil))
	srvAssets := startPlain(t, newRecorder(nil))
	srvMarketplace := startPlain(t, newRecorder(nil))
	rec := recordAt(srvDeployment.URL)
	rec.AssetsAPIURL = srvAssets.URL
	rec.MarketplaceAPIURL = srvMarketplace.URL
	srv := recordServer(t, rec)
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	_, err := r.Region(t.Context(), "aws.euw1")

	require.NoError(t, err)
	assert.True(t, r.core.isPlatformHost(mustURL(t, srvDeployment.URL)))
	assert.True(t, r.core.isPlatformHost(mustURL(t, srvMarketplace.URL)))
	assert.False(t, r.core.isPlatformHost(mustURL(t, srvAssets.URL)))
}
