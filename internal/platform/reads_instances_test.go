package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

const instanceBody = `{"id":"i-1","name":"dev","serviceName":"my-project-dev","apiApplicationId":"app-1","projectId":"proj-1","regionAlias":"aws.euw1"}`

var wantInstance = Instance{
	ID: "i-1", Name: "dev", ServiceName: "my-project-dev", APIApplicationID: "app-1", ProjectID: "proj-1", RegionAlias: "aws.euw1",
}

func TestGetInstance(t *testing.T) {
	t.Run("decodes the instance", func(t *testing.T) {
		rec := newRecorder(answerJSON(http.StatusOK, instanceBody))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		got, err := c.GetInstance(t.Context(), "i-1")

		require.NoError(t, err)
		assert.Equal(t, wantInstance, got)
		req := rec.requests()[0]
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/v0.3/instances/i-1", req.RawPath)
		assert.Empty(t, req.RawQuery)
		assertCredentialsSent(t, req.Header)
	})

	t.Run("unsafe ids send nothing", func(t *testing.T) {
		rec := newRecorder(nil)
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		for _, id := range []string{"", ".", "..", "a/b", "a%2Fb"} {
			_, err := c.GetInstance(t.Context(), id)
			require.Error(t, err, "GetInstance(%q)", id)
		}

		assert.Zero(t, rec.count())
	})

	t.Run("an id from the server that is not safe fails the call", func(t *testing.T) {
		body := `{"id":"../x","name":"dev","serviceName":"s","apiApplicationId":"a","projectId":"p","regionAlias":"aws.euw1"}`
		srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, body)))
		c, _, _ := newTestClient(t, srv)

		got, err := c.GetInstance(t.Context(), "i-1")

		require.EqualError(t, err, "the platform returned an invalid instance id: ../x")
		assert.Equal(t, Instance{}, got)
	})

	t.Run("a 404 envelope is not found", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeInstanceNotFound)))
		c, _, _ := newTestClient(t, srv)

		_, err := c.GetInstance(t.Context(), "i-9")

		requireNotFound(t, err, 2003)
	})
}

func TestGetInstanceByName(t *testing.T) {
	t.Run("decodes the instance", func(t *testing.T) {
		rec := newRecorder(answerJSON(http.StatusOK, instanceBody))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		got, err := c.GetInstanceByName(t.Context(), "p", "n")

		require.NoError(t, err)
		assert.Equal(t, wantInstance, got)
		req := rec.requests()[0]
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/v0.3/projects/p/instances/n", req.RawPath)
		assertCredentialsSent(t, req.Header)
	})

	t.Run("segments are escaped", func(t *testing.T) {
		rec := newRecorder(answerJSON(http.StatusOK, instanceBody))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		_, err := c.GetInstanceByName(t.Context(), "my project", "a b")

		require.NoError(t, err)
		assert.Equal(t, "/v0.3/projects/my%20project/instances/a%20b", rec.requests()[0].RawPath)
	})

	t.Run("unsafe names send nothing", func(t *testing.T) {
		rec := newRecorder(nil)
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		for _, pair := range [][2]string{{"", "n"}, {"p", ""}, {"..", "n"}, {"p", ".."}, {"a/b", "n"}, {"p", "a/b"}} {
			_, err := c.GetInstanceByName(t.Context(), pair[0], pair[1])
			require.Error(t, err, "GetInstanceByName(%q, %q)", pair[0], pair[1])
		}

		assert.Zero(t, rec.count())
	})

	t.Run("an id from the server that is not safe fails the call", func(t *testing.T) {
		body := `{"id":"a b","name":"dev","serviceName":"s","apiApplicationId":"a","projectId":"p","regionAlias":"aws.euw1"}`
		srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, body)))
		c, _, _ := newTestClient(t, srv)

		_, err := c.GetInstanceByName(t.Context(), "p", "n")

		require.EqualError(t, err, "the platform returned an invalid instance id: a b")
	})

	t.Run("a 404 envelope is not found", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeInstanceNotFound)))
		c, _, _ := newTestClient(t, srv)

		_, err := c.GetInstanceByName(t.Context(), "p", "n")

		requireNotFound(t, err, 2003)
	})
}

const logsBody = `{"logs":[` +
	`{"id":"l-3","logLevel":"error","sourceType":"provider","message":"provider call failed","timestamp":"2026-10-14T10:11:12.123456789Z"},` +
	`{"id":"l-2","logLevel":"warn","sourceType":"application","message":"slow request","timestamp":"2026-10-14T10:11:12Z"},` +
	`{"id":"l-1","logLevel":"info","sourceType":"application","message":"starting","timestamp":"2026-10-14T10:11:11.5Z"}]}`

func TestListLogs(t *testing.T) {
	t.Run("without a cursor only the limit is sent", func(t *testing.T) {
		rec := newRecorder(answerJSON(http.StatusOK, logsBody))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		got, err := c.ListLogs(t.Context(), "i-1", 300, "")

		require.NoError(t, err)
		req := rec.requests()[0]
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/v0.3/instances/i-1/logs", req.RawPath)
		assert.Equal(t, "limit=300", req.RawQuery)
		assertCredentialsSent(t, req.Header)
		assert.Equal(t, []LogEntry{
			{ID: "l-3", LogLevel: "error", SourceType: "provider", Message: "provider call failed", Timestamp: "2026-10-14T10:11:12.123456789Z"},
			{ID: "l-2", LogLevel: "warn", SourceType: "application", Message: "slow request", Timestamp: "2026-10-14T10:11:12Z"},
			{ID: "l-1", LogLevel: "info", SourceType: "application", Message: "starting", Timestamp: "2026-10-14T10:11:11.5Z"},
		}, got, "newest first, and the timestamps are the server's own strings")
	})

	t.Run("the cursor is sent unchanged", func(t *testing.T) {
		rec := newRecorder(answerJSON(http.StatusOK, `{"logs":[]}`))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		_, err := c.ListLogs(t.Context(), "i-1", 300, "2026-10-14T10:11:12.123456789Z")

		require.NoError(t, err)
		assert.Equal(t, "after=2026-10-14T10%3A11%3A12.123456789Z&limit=300", rec.requests()[0].RawQuery)
	})

	t.Run("a 404 envelope is not found", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeInstanceNotFound)))
		c, _, _ := newTestClient(t, srv)

		_, err := c.ListLogs(t.Context(), "i-9", 300, "")

		requireNotFound(t, err, 2003)
	})

	t.Run("a logs backend failure is an error, never an empty list", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusServiceUnavailable, envelopeUpstream)))
		c, _, _ := newTestClient(t, srv)

		got, err := c.ListLogs(t.Context(), "i-1", 300, "")

		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
		assert.Equal(t, 9001, apiErr.ServerCode)
		assert.NotErrorIs(t, err, ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("unsafe ids send nothing", func(t *testing.T) {
		rec := newRecorder(nil)
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		for _, id := range []string{"", "..", "a/b"} {
			_, err := c.ListLogs(t.Context(), id, 300, "")
			require.Error(t, err, "ListLogs(%q)", id)
		}

		assert.Zero(t, rec.count())
	})
}

func TestReadRoutesNeverRetry(t *testing.T) {
	for _, call := range readCalls() {
		t.Run(call.name, func(t *testing.T) {
			rec := newRecorder(answerJSON(http.StatusServiceUnavailable, envelopeUpstream))
			srv := startPlain(t, rec)

			err := call.run(t, srv)

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
			assert.Equal(t, 1, rec.count())
		})
	}
}
