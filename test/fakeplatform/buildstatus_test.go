package fakeplatform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildStatusOf(t *testing.T, s *Server, packageID string) reply {
	t.Helper()
	return rest(t, s, http.MethodGet, "/packages/"+packageID+"/build/status", nil)
}

func TestBuildStatusDefaultsToCompleted(t *testing.T) {
	s := Start(t, Defaults())
	packageID := Defaults().Deploy.PackageID

	r := buildStatusOf(t, s, packageID)

	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	assert.Equal(t, map[string]any{"packageId": packageID, "status": "completed"}, r.object(t))
	reqs := s.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "GET /packages/{id}/build/status", reqs[0].Route)
}

func TestBuildStatusAnswersInOrder(t *testing.T) {
	s := started(t, "buildStatus: [{status: pending}, {http: 503}, {status: building}, {status: failed}]\n")
	packageID := Defaults().Deploy.PackageID

	first := buildStatusOf(t, s, packageID)
	assert.Equal(t, "pending", first.object(t)["status"])

	unavailable := buildStatusOf(t, s, packageID)
	require.Equal(t, http.StatusServiceUnavailable, unavailable.status)
	code, _ := unavailable.apiError(t)
	assert.Equal(t, http.StatusServiceUnavailable, code, "an HTTP answer uses the error envelope")

	assert.Equal(t, "building", buildStatusOf(t, s, packageID).object(t)["status"])
	assert.Equal(t, "failed", buildStatusOf(t, s, packageID).object(t)["status"])
	assert.Equal(t, "failed", buildStatusOf(t, s, packageID).object(t)["status"], "the last answer repeats")
}

func TestBuildStatusUnknownPackage(t *testing.T) {
	s := Start(t, Defaults())

	notFound(t, buildStatusOf(t, s, fixtureID(799)), 2006)
	notFound(t, buildStatusOf(t, s, "not-a-uuid"), 2006)
}
