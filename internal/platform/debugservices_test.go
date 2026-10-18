package platform

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

func debugServiceReply(serviceName, instanceID string) string {
	return mustJSON(map[string]string{
		"serviceName": serviceName, "privateKey": "pk", "instanceId": instanceID, "websocketPath": "abc123",
	})
}

func TestDeployDebugServiceBody(t *testing.T) {
	caps := manifest.Capabilities{Messages: "v0.1"}
	tests := []struct {
		name     string
		service  string
		wantBody string
	}{
		{
			name:     "without a name the key is left out",
			wantBody: `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","capabilities":{"messages":"v0.1"}}`,
		},
		{
			name:     "a name sits between the application and the capabilities",
			service:  "dbg",
			wantBody: `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","name":"dbg","capabilities":{"messages":"v0.1"}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply("dbg-svc-1", "inst-1")))

			got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", tc.service, caps)

			require.NoError(t, err)
			assert.Equal(t, DebugService{ServiceName: "dbg-svc-1", PrivateKey: "pk", InstanceID: "inst-1", WebsocketPath: "abc123"}, got)
			req := s.only(t)
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "/v0.3/debug/services", req.RawPath)
			assert.Empty(t, req.RawQuery)
			assert.Equal(t, tc.wantBody, string(req.Body))
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestDeployDebugServiceNoCapabilitiesIsAnEmptyObject(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply("dbg-svc-1", "inst-1")))

	_, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{})

	require.NoError(t, err)
	assert.Equal(t, `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","capabilities":{}}`, string(s.only(t).Body))
}

func TestDeployDebugServiceChecksServerNames(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		instanceID  string
		wantError   string
	}{
		{name: "a dot", serviceName: "a.b", instanceID: "inst-1", wantError: "the platform returned an invalid service name: a.b"},
		{name: "a leading hyphen", serviceName: "-a", instanceID: "inst-1", wantError: "the platform returned an invalid service name: -a"},
		{name: "a traversal", serviceName: "../x", instanceID: "inst-1", wantError: "the platform returned an invalid service name: ../x"},
		{name: "a name over 63 characters", serviceName: strings.Repeat("a", 64), instanceID: "inst-1", wantError: "invalid service name"},
		{name: "an empty name", serviceName: "", instanceID: "inst-1", wantError: "invalid service name"},
		{name: "an instance id with a traversal", serviceName: "dbg-svc-1", instanceID: "../x", wantError: "the platform returned an invalid instance id: ../x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply(tc.serviceName, tc.instanceID)))

			got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantError)
			assert.Empty(t, got, "a rejected service is not returned")
		})
	}
}

func TestDeployDebugServiceAcceptsNoInstanceID(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply("dbg-svc-1", "")))

	got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{})

	require.NoError(t, err)
	assert.Equal(t, DebugService{ServiceName: "dbg-svc-1", PrivateKey: "pk", WebsocketPath: "abc123"}, got,
		"the debugged app then runs without INSTANCE_ID")
}

func TestDeployDebugServiceAcceptsANameEndingInAHyphen(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply("my-service-", "00000000-0000-4000-8000-000000000771")))

	got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{})

	require.NoError(t, err)
	assert.Equal(t, "my-service-", got.ServiceName)
}

func TestDeployDebugServiceFailureIsAnAPIError(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusBadRequest, `{"error":{"code":5001,"message":"unknown region"}}`))

	_, err := s.client.DeployDebugService(t.Context(), "aws.nowhere", "app-1", "", manifest.Capabilities{})

	apiErr := requireAPIError(t, err, http.StatusBadRequest)
	assert.Equal(t, "unknown region", apiErr.Message)
}

func TestDebugServiceReady(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(mustJSON(ready), func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, mustJSON(map[string]bool{"ready": ready})))

			got, err := s.client.DebugServiceReady(t.Context(), "svc-1")

			require.NoError(t, err)
			assert.Equal(t, ready, got)
			req := s.only(t)
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Equal(t, "/v0.3/debug/services/svc-1/status", req.RawPath)
			assert.Empty(t, req.RawQuery)
			assert.Empty(t, req.Body)
		})
	}

	t.Run("a failed call is an API error", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusNotFound, `{"error":{"code":5002,"message":"no such service"}}`))

		got, err := s.client.DebugServiceReady(t.Context(), "svc-1")

		requireAPIError(t, err, http.StatusNotFound)
		assert.False(t, got)
	})

	t.Run("the name is checked", func(t *testing.T) {
		s := newRouteServer(t, nil)

		for _, name := range []string{"", "..", "../x", "a/b"} {
			_, err := s.client.DebugServiceReady(t.Context(), name)
			require.Error(t, err)
		}
		assert.Zero(t, s.rec.count())
	})
}

func TestDeleteDebugServiceQuery(t *testing.T) {
	for _, preserve := range []bool{true, false} {
		t.Run(mustJSON(preserve), func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, `{}`))

			require.NoError(t, s.client.DeleteDebugService(t.Context(), "svc-1", preserve))

			req := s.only(t)
			assert.Equal(t, http.MethodDelete, req.Method)
			assert.Equal(t, "/v0.3/debug/services/svc-1", req.RawPath)
			assert.Equal(t, "preserveData="+mustJSON(preserve), req.RawQuery)
			assert.Empty(t, req.Body)
		})
	}

	t.Run("a failed call is an API error", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusInternalServerError, `{"error":{"code":1,"message":"boom"}}`))

		err := s.client.DeleteDebugService(t.Context(), "svc-1", false)

		requireAPIError(t, err, http.StatusInternalServerError)
	})
}

func TestDeleteDebugServiceEmptyNameSendsNothing(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../x", "a/b"} {
		t.Run(name, func(t *testing.T) {
			s := newRouteServer(t, nil)

			err := s.client.DeleteDebugService(t.Context(), name, false)

			require.Error(t, err)
			assert.Zero(t, s.rec.count(), "an empty name would have deleted every debug service")
		})
	}
}

func TestPruneDebugSessions(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusOK, `{}`))

	require.NoError(t, s.client.PruneDebugSessions(t.Context()))

	req := s.only(t)
	assert.Equal(t, http.MethodDelete, req.Method)
	assert.Equal(t, "/v0.3/debug/services", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assert.Empty(t, req.Body)
	assertCredentialsSent(t, req.Header)
}

func TestPruneDebugSessionsFailureIsAnAPIError(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusForbidden, `{"error":{"code":403,"message":"forbidden"}}`))

	err := s.client.PruneDebugSessions(t.Context())

	requireAPIError(t, err, http.StatusForbidden)
}
