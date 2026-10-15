package platform

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIErrorSanitisedAndCapped(t *testing.T) {
	c := errorCore(t)

	t.Run("control sequences are removed", func(t *testing.T) {
		got := c.apiError(http.StatusBadRequest, nil, []byte("\x1b]0;t\a\x1b[31mred\x1b[0m"), "h", nil)

		assert.NotContains(t, got.Message, "\x1b")
		assert.Equal(t, "red", got.Message)
	})

	t.Run("long text is cut on a rune boundary", func(t *testing.T) {
		got := c.apiError(http.StatusBadRequest, nil, []byte(strings.Repeat("é", 3000)), "h", nil)

		assert.True(t, utf8.ValidString(got.Message))
		assert.True(t, strings.HasSuffix(got.Message, "…"))
		assert.LessOrEqual(t, len(got.Message), 2051)
		assert.Greater(t, len(got.Message), 2000)
	})

	t.Run("a cut inside a rune backs up to its start", func(t *testing.T) {
		got := c.apiError(http.StatusBadRequest, nil, []byte("a"+strings.Repeat("é", 3000)), "h", nil)

		assert.True(t, utf8.ValidString(got.Message))
		assert.True(t, strings.HasSuffix(got.Message, "é…"))
		assert.LessOrEqual(t, len(got.Message), 2051)
	})

	t.Run("text at the limit is kept whole", func(t *testing.T) {
		text := strings.Repeat("a", 2048)

		got := c.apiError(http.StatusBadRequest, nil, []byte(text), "h", nil)

		assert.Equal(t, text, got.Message)
	})

	t.Run("a json string body is plain text", func(t *testing.T) {
		got := c.apiError(http.StatusBadRequest, nil, []byte(`"oops"`), "h", nil)

		assert.Equal(t, `"oops"`, got.Message)
		assert.Zero(t, got.ServerCode)
	})

	t.Run("invalid utf-8 is replaced", func(t *testing.T) {
		got := c.apiError(http.StatusBadRequest, nil, []byte("bad \xff\xfe bytes"), "h", nil)

		assert.True(t, utf8.ValidString(got.Message))
	})
}

func TestAPIErrorRedactsSecrets(t *testing.T) {
	const secret = "s3cret-for-tests"
	encoded := base64.StdEncoding.EncodeToString([]byte("abc123:" + secret))
	c := errorCore(t)

	t.Run("plain body", func(t *testing.T) {
		body := "echo: Authorization: Basic " + encoded + " and " + secret

		got := c.apiError(http.StatusBadRequest, nil, []byte(body), "h", nil)

		assert.Equal(t, "echo: Authorization: Basic *** and ***", got.Message)
	})

	t.Run("a secret split by a control sequence is still found", func(t *testing.T) {
		body := "x s3cret\x1b[0m-for-tests y"

		got := c.apiError(http.StatusBadRequest, nil, []byte(body), "h", nil)

		assert.Equal(t, "x *** y", got.Message)
	})

	t.Run("envelope message and container logs", func(t *testing.T) {
		body := `{"error":{"code":1,"message":"bad ` + secret + `","containerLogs":"line ` + encoded + `\nnext"}}`

		got := c.apiError(http.StatusBadRequest, nil, []byte(body), "h", nil)

		assert.Equal(t, "bad ***", got.Message)
		assert.Equal(t, "line ***\nnext", got.ContainerLogs)
	})

	t.Run("html summary", func(t *testing.T) {
		got := c.apiError(http.StatusBadGateway, headerWith("Content-Type", "text/html"), []byte("<p>x</p>"), secret, nil)

		assert.Equal(t, "HTTP 502 from ***", got.Message)
	})

	t.Run("the redaction happens before the cut", func(t *testing.T) {
		body := strings.Repeat("a", 2040) + secret

		got := c.apiError(http.StatusBadRequest, nil, []byte(body), "h", nil)

		assert.NotContains(t, got.Message, "s3cret")
		assert.Equal(t, strings.Repeat("a", 2040)+"***", got.Message)
	})

	t.Run("an empty secret changes nothing", func(t *testing.T) {
		noSecret, err := newCore(Options{
			Bootstrap: "https://api.example.test",
			Creds:     Credentials{APIKey: "abc123"},
		}, nil)
		require.NoError(t, err)
		body := `{"error":{"code":1,"message":"plain text","containerLogs":"logs"}}`

		got := noSecret.apiError(http.StatusBadRequest, nil, []byte(body), "h", nil)

		assert.Equal(t, "plain text", got.Message)
		assert.Equal(t, "logs", got.ContainerLogs)
		assert.Equal(t, "no secret here", noSecret.apiError(http.StatusBadRequest, nil, []byte("no secret here"), "h", nil).Message)
	})
}

func TestAPIErrorIgnoresMalformedTraceHeader(t *testing.T) {
	c := errorCore(t)
	for _, bad := range []string{"bad value\x1b[31m", "", strings.Repeat("a", 129), "a b", "../x\n"} {
		t.Run(bad, func(t *testing.T) {
			got := c.apiError(http.StatusBadRequest, headerWith("X-Neru-Traceid", bad), []byte("x"), "h", recordOf("req-1"))

			assert.Equal(t, "req-1", got.TraceID)
		})
	}

	valid := c.apiError(http.StatusBadRequest, headerWith("X-Neru-Traceid", strings.Repeat("a", 128)), []byte("x"), "h", recordOf("req-1"))
	assert.Equal(t, strings.Repeat("a", 128), valid.TraceID)
}
