package platform

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
)

func olderPlatformMessage(label string) string {
	return "this region's platform API is older than this CLI and does not provide " + label +
		"; try again later or use an earlier vcr release"
}

func TestPlainNotFoundOnNewRoutesExplainsOlderPlatform(t *testing.T) {
	bodies := []struct {
		name        string
		contentType string
		body        string
	}{
		{"framework page", "text/plain; charset=utf-8", "404 page not found\n"},
		{"html gateway page", "text/html; charset=utf-8", "<html><body><h1>404 Not Found</h1></body></html>"},
	}
	for _, call := range readCalls() {
		for _, b := range bodies {
			t.Run(call.name+"/"+b.name, func(t *testing.T) {
				srv := startPlain(t, newRecorder(answer(http.StatusNotFound, b.contentType, b.body)))

				err := call.run(t, srv)

				var apiErr *clierr.APIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
				assert.Zero(t, apiErr.ServerCode)
				assert.Equal(t, serverTraceID, apiErr.TraceID)
				assert.Equal(t, olderPlatformMessage(call.label), apiErr.Message)
				assert.NotErrorIs(t, err, ErrNotFound)
			})
		}
	}

	t.Run("a 404 envelope keeps the server's own message", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, `{"error":{"code":2004,"message":"runtime not found"}}`)))
		c, _, _ := newTestClient(t, srv)

		_, err := c.ListRuntimes(t.Context())

		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 2004, apiErr.ServerCode)
		assert.Equal(t, "runtime not found", apiErr.Message)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestNotFoundThatIsNotTheEnvelopeExplainsOlderPlatform(t *testing.T) {
	bodies := map[string]string{
		"json without the envelope": `{"message":"no Route matched with those values"}`,
		"envelope with code zero":   `{"error":{"code":0,"message":"not found"}}`,
		"empty object":              `{}`,
		"empty body":                ``,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, body)))
			c, _, _ := newTestClient(t, srv)

			_, err := c.GetProject(t.Context(), "my-project")

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, olderPlatformMessage("GET /projects/{name}"), apiErr.Message)
			assert.NotErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestNotFoundEnvelopeMapsOnlyOnRoutesThatLookARecordUp(t *testing.T) {
	const envelope = `{"error":{"code":2099,"message":"gone"}}`
	for _, call := range readCalls() {
		t.Run(call.name, func(t *testing.T) {
			srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelope)))

			err := call.run(t, srv)

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
			assert.Equal(t, 2099, apiErr.ServerCode)
			assert.Equal(t, "gone", apiErr.Message, "the server's own message, with no older-platform text")
			assert.Equal(t, serverTraceID, apiErr.TraceID)
			if call.notFound {
				assert.ErrorIs(t, err, ErrNotFound)
			} else {
				assert.NotErrorIs(t, err, ErrNotFound)
			}
		})
	}
}

func TestReadErrorLeavesOtherFailuresAlone(t *testing.T) {
	apiErr := &clierr.APIError{HTTPStatus: http.StatusInternalServerError, ServerCode: 1001, Message: "boom"}
	for name, err := range map[string]error{
		"network failure": io.ErrUnexpectedEOF,
		"server error":    apiErr,
		"wrapped":         fmt.Errorf("context: %w", apiErr),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, err, readError(routeProject, err))
		})
	}
}

func TestReadErrorDoesNotChangeTheErrorItIsGiven(t *testing.T) {
	original := &clierr.APIError{HTTPStatus: http.StatusNotFound, Message: "404 page not found", TraceID: "t-1"}

	got := readError(routeRuntimes, fmt.Errorf("wrapped: %w", original))

	var older *clierr.APIError
	require.ErrorAs(t, got, &older)
	assert.Equal(t, olderPlatformMessage("GET /runtimes"), older.Message)
	assert.Equal(t, "t-1", older.TraceID)
	assert.Equal(t, "404 page not found", original.Message)
	assert.NotSame(t, original, older)
}
