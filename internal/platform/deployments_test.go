package platform

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

func TestCreateProjectBody(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusOK, `{"projectId":"proj-1"}`))

	id, err := s.client.CreateProject(t.Context(), "p")

	require.NoError(t, err)
	assert.Equal(t, "proj-1", id)
	req := s.only(t)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "/v0.3/projects", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assert.Equal(t, `{"name":"p","origin":"owner"}`, string(req.Body))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assertCredentialsSent(t, req.Header)
}

func TestCreateProjectFailureIsAnAPIError(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusConflict, `{"error":{"code":2011,"message":"project exists"}}`))

	_, err := s.client.CreateProject(t.Context(), "p")

	apiErr := apiErrorWithStatus(t, err, http.StatusConflict)
	assert.Equal(t, 2011, apiErr.ServerCode)
}

func TestCreatePackageBodyAndID(t *testing.T) {
	tests := []struct {
		name     string
		req      PackageRequest
		wantBody string
	}{
		{
			name: "no entrypoint is null",
			req: PackageRequest{
				SourceCodeKey: "k1",
				Capabilities:  manifest.Capabilities{Messages: "v0.1", Voice: "v0"},
				Runtime:       "nodejs18",
			},
			wantBody: `{"sourceCodeKey":"k1","entrypoint":null,"capabilities":{"messages":"v0.1","voice":"v0"},"buildScriptPath":"","runtime":"nodejs18"}`,
		},
		{
			name: "every field",
			req: PackageRequest{
				SourceCodeKey:   "k2",
				Entrypoint:      []string{"node", "index.js"},
				Capabilities:    manifest.Capabilities{RTC: "v0"},
				BuildScriptPath: "build.sh",
				Runtime:         "python3",
			},
			wantBody: `{"sourceCodeKey":"k2","entrypoint":["node","index.js"],"capabilities":{"rtc":"v0"},"buildScriptPath":"build.sh","runtime":"python3"}`,
		},
		{
			name:     "no capabilities is an empty object",
			req:      PackageRequest{SourceCodeKey: "k3", Runtime: "nodejs18"},
			wantBody: `{"sourceCodeKey":"k3","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, `{"packageId":"pkg-1"}`))

			id, err := s.client.CreatePackage(t.Context(), tc.req)

			require.NoError(t, err)
			assert.Equal(t, "pkg-1", id)
			req := s.only(t)
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "/v0.3/packages", req.RawPath)
			assert.Equal(t, tc.wantBody, string(req.Body))
		})
	}
}

func TestCreatePackageChecksTheReturnedID(t *testing.T) {
	for _, id := range []string{"../x", "", "a b", "a/b"} {
		t.Run(id, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, mustJSON(map[string]string{"packageId": id})))

			got, err := s.client.CreatePackage(t.Context(), PackageRequest{SourceCodeKey: "k1"})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "the platform returned an invalid package id: "+id)
			assert.Empty(t, got)
		})
	}
}

func TestValidateDeploymentResult(t *testing.T) {
	req := ValidateRequest{ProjectID: "proj-1", APIApplicationID: "app-1", InstanceName: "dev", Region: "aws.euw1"}
	const wantBody = `{"projectId":"proj-1","apiApplicationId":"app-1","instanceName":"dev","region":"aws.euw1",` +
		`"environment":null,"minScale":0,"maxScale":0}`

	t.Run("a result with problems is not an error", func(t *testing.T) {
		reply := `{"valid":false,"errors":[{"field":"environment[0].name","message":"reserved name"}]}`
		s := newRouteServer(t, answerJSON(http.StatusOK, reply))

		got, err := s.client.ValidateDeployment(t.Context(), req)

		require.NoError(t, err)
		assert.Equal(t, ValidateResult{
			Valid:  false,
			Errors: []ValidationError{{Field: "environment[0].name", Message: "reserved name"}},
		}, got)
		recorded := s.only(t)
		assert.Equal(t, http.MethodPost, recorded.Method)
		assert.Equal(t, "/v0.3/deployments/validate", recorded.RawPath)
		assert.Equal(t, wantBody, string(recorded.Body))
	})

	t.Run("a valid result", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, `{"valid":true}`))

		got, err := s.client.ValidateDeployment(t.Context(), req)

		require.NoError(t, err)
		assert.Equal(t, ValidateResult{Valid: true}, got)
	})

	t.Run("environment and scale are sent as given", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, `{"valid":true}`))
		full := req
		full.Environment = []manifest.Env{{Name: "A", Value: "1"}, {Name: "B", Secret: "TOKEN"}}
		full.MinScale, full.MaxScale = 1, 3

		_, err := s.client.ValidateDeployment(t.Context(), full)

		require.NoError(t, err)
		assert.Equal(t, `{"projectId":"proj-1","apiApplicationId":"app-1","instanceName":"dev","region":"aws.euw1",`+
			`"environment":[{"name":"A","value":"1","secret":""},{"name":"B","value":"","secret":"TOKEN"}],`+
			`"minScale":1,"maxScale":3}`, string(s.only(t).Body))
	})

	t.Run("a failed call is an API error", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusBadRequest, `{"error":{"code":3001,"message":"bad request"}}`))

		_, err := s.client.ValidateDeployment(t.Context(), req)

		requireAPIError(t, err, http.StatusBadRequest)
	})
}

func TestDeployInstanceBody(t *testing.T) {
	const reply = `{"instanceId":"inst-1","serviceName":"my-project-dev","deploymentId":"dep-1","hostUrls":["https://a.example.test","https://b.example.test"]}`
	want := DeployResult{
		InstanceID: "inst-1", ServiceName: "my-project-dev", DeploymentID: "dep-1",
		HostURLs: []string{"https://a.example.test", "https://b.example.test"},
	}
	tests := []struct {
		name     string
		req      DeployRequest
		wantBody string
	}{
		{
			name: "nothing optional",
			req: DeployRequest{
				PackageID: "pkg-1", ProjectID: "proj-1", APIApplicationID: "app-1", InstanceName: "dev", Region: "aws.euw1",
			},
			wantBody: `{"packageId":"pkg-1","projectId":"proj-1","apiApplicationId":"app-1","instanceName":"dev","region":"aws.euw1",` +
				`"environment":null,"domains":null,"minScale":0,"maxScale":0}`,
		},
		{
			name: "everything set",
			req: DeployRequest{
				PackageID: "pkg-1", ProjectID: "proj-1", APIApplicationID: "app-1", InstanceName: "dev", Region: "aws.euw1",
				Environment:         []manifest.Env{{Name: "A", Value: "1"}},
				Domains:             []string{"x.example.test"},
				MinScale:            1,
				MaxScale:            2,
				Security:            &manifest.Security{Access: "private"},
				HealthCheckEndpoint: "/health",
			},
			wantBody: `{"packageId":"pkg-1","projectId":"proj-1","apiApplicationId":"app-1","instanceName":"dev","region":"aws.euw1",` +
				`"environment":[{"name":"A","value":"1","secret":""}],"domains":["x.example.test"],"minScale":1,"maxScale":2,` +
				`"security":{"access":"private"},"healthCheckEndpoint":"/health"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, reply))

			got, err := s.client.DeployInstance(t.Context(), tc.req)

			require.NoError(t, err)
			assert.Equal(t, want, got)
			req := s.only(t)
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "/v0.3/deployments", req.RawPath)
			assert.Equal(t, tc.wantBody, string(req.Body))
		})
	}
}

func TestDeployInstanceFailureIsAnAPIError(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusUnprocessableEntity, `{"error":{"code":3002,"message":"cannot deploy"}}`))

	_, err := s.client.DeployInstance(t.Context(), DeployRequest{PackageID: "pkg-1"})

	apiErr := apiErrorWithStatus(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, "cannot deploy", apiErr.Message)
}

func TestDeleteInstancePathAndConflict(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, `{}`))

		require.NoError(t, s.client.DeleteInstance(t.Context(), "inst-1"))

		req := s.only(t)
		assert.Equal(t, http.MethodDelete, req.Method)
		assert.Equal(t, "/v0.3/instances/inst-1", req.RawPath)
		assert.Empty(t, req.RawQuery)
		assert.Empty(t, req.Body)
	})

	t.Run("a region mismatch is an API error and no sentinel", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusConflict, `{"error":{"code":2012,"message":"instance lives in another region"}}`))

		err := s.client.DeleteInstance(t.Context(), "inst-1")

		apiErr := apiErrorWithStatus(t, err, http.StatusConflict)
		assert.Equal(t, "instance lives in another region", apiErr.Message)
		assert.NotErrorIs(t, err, ErrNotFound)
		assert.NotErrorIs(t, err, ErrAlreadyExists)
	})

	t.Run("a missing instance is an API error too", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusNotFound, `{"error":{"code":2003,"message":"instance not found"}}`))

		err := s.client.DeleteInstance(t.Context(), "inst-1")

		requireAPIError(t, err, http.StatusNotFound)
		assert.NotErrorIs(t, err, ErrNotFound)
	})

	t.Run("the id is checked", func(t *testing.T) {
		s := newRouteServer(t, nil)

		for _, id := range []string{"", "..", "a/b"} {
			require.Error(t, s.client.DeleteInstance(t.Context(), id))
		}
		assert.Zero(t, s.rec.count())
	})
}

// neverRetried lists every call except the upload, each as a function that makes it with valid arguments.
func neverRetried() map[string]func(ctx context.Context, c *Client) error {
	return map[string]func(ctx context.Context, c *Client) error{
		"CreateApplication": func(ctx context.Context, c *Client) error {
			_, err := c.CreateApplication(ctx, "a", true, true, true)
			return err
		},
		"ListApplications": func(ctx context.Context, c *Client) error {
			_, err := c.ListApplications(ctx, "")
			return err
		},
		"GenerateApplicationKeys": func(ctx context.Context, c *Client) error { return c.GenerateApplicationKeys(ctx, "app-1") },
		"DeleteApplication":       func(ctx context.Context, c *Client) error { return c.DeleteApplication(ctx, "app-1") },
		"CreateSecret":            func(ctx context.Context, c *Client) error { return c.CreateSecret(ctx, "N", "v") },
		"UpdateSecret":            func(ctx context.Context, c *Client) error { return c.UpdateSecret(ctx, "N", "v") },
		"DeleteSecret":            func(ctx context.Context, c *Client) error { return c.DeleteSecret(ctx, "N") },
		"ListSecrets": func(ctx context.Context, c *Client) error {
			_, err := c.ListSecrets(ctx)
			return err
		},
		"CreateProject": func(ctx context.Context, c *Client) error {
			_, err := c.CreateProject(ctx, "p")
			return err
		},
		"CreatePackage": func(ctx context.Context, c *Client) error {
			_, err := c.CreatePackage(ctx, PackageRequest{SourceCodeKey: "k"})
			return err
		},
		"ValidateDeployment": func(ctx context.Context, c *Client) error {
			_, err := c.ValidateDeployment(ctx, ValidateRequest{})
			return err
		},
		"DeployInstance": func(ctx context.Context, c *Client) error {
			_, err := c.DeployInstance(ctx, DeployRequest{})
			return err
		},
		"DeleteInstance": func(ctx context.Context, c *Client) error { return c.DeleteInstance(ctx, "inst-1") },
		"DeployDebugService": func(ctx context.Context, c *Client) error {
			_, err := c.DeployDebugService(ctx, "aws.euw1", "app-1", "", manifest.Capabilities{})
			return err
		},
		"DebugServiceReady": func(ctx context.Context, c *Client) error {
			_, err := c.DebugServiceReady(ctx, "svc-1")
			return err
		},
		"DeleteDebugService": func(ctx context.Context, c *Client) error { return c.DeleteDebugService(ctx, "svc-1", false) },
		"PruneDebugSessions": func(ctx context.Context, c *Client) error { return c.PruneDebugSessions(ctx) },
	}
}

func TestWriteCallsNeverRetry(t *testing.T) {
	failures := map[string]func(http.ResponseWriter, *http.Request){
		"service unavailable": answerJSON(http.StatusServiceUnavailable, `{"error":{"code":1,"message":"busy"}}`),
		"hang up":             hangUp,
	}
	for name, call := range neverRetried() {
		for failure, handler := range failures {
			t.Run(name+" "+failure, func(t *testing.T) {
				s := newRouteServer(t, handler)

				err := call(t.Context(), s.client)

				require.Error(t, err)
				assert.Equal(t, 1, s.rec.count(), "one request, no retry")
				assert.Empty(t, s.clock.Sleeps())
				assert.Empty(t, s.notices.String())
			})
		}
	}
}
