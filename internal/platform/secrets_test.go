package platform

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// secretBody is what CreateSecret and UpdateSecret send for the value a"b<c: json.Marshal writes < as a six
// character escape, and the escape is built in two parts so that no editor or tool turns it back into a <.
const secretBody = `{"secrets":[{"name":"API_KEY","value":"a\"b` + `\` + `u003cc"}]}`

func TestSecretRoutesWireFormat(t *testing.T) {
	tests := []struct {
		name       string
		reply      string
		call       func(ctx context.Context, c *Client) (any, error)
		wantMethod string
		wantPath   string
		wantBody   string
		want       any
	}{
		{
			name:  "create",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.CreateSecret(ctx, "API_KEY", `a"b<c`)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v0.3/secrets",
			wantBody:   secretBody,
		},
		{
			name:  "update uses PATCH and the same shape",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.UpdateSecret(ctx, "API_KEY", `a"b<c`)
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/v0.3/secrets",
			wantBody:   secretBody,
		},
		{
			name:  "delete",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.DeleteSecret(ctx, "API_KEY")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v0.3/secrets/API_KEY",
		},
		{
			name:  "delete escapes the name",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.DeleteSecret(ctx, "my secret")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v0.3/secrets/my%20secret",
		},
		{
			name:  "list",
			reply: `{"secrets":["a","b"]}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ListSecrets(ctx)
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v0.3/secrets",
			want:       []string{"a", "b"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, tc.reply))

			got, err := tc.call(t.Context(), s.client)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			req := s.only(t)
			assert.Equal(t, tc.wantMethod, req.Method)
			assert.Equal(t, tc.wantPath, req.RawPath)
			assert.Empty(t, req.RawQuery)
			assert.Equal(t, tc.wantBody, string(req.Body))
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestCreateSecretConflict(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusConflict, `{"error":{"code":2009,"message":"secret exists"}}`))

	err := s.client.CreateSecret(t.Context(), "API_KEY", "v")

	assert.Equal(t, ErrAlreadyExists, err, "the sentinel comes back bare")
	assert.Equal(t, 1, s.rec.count())
}

func TestCreateSecretOtherFailuresAreAPIErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newRouteServer(t, answerJSON(status, `{"error":{"code":1,"message":"nope"}}`))

			err := s.client.CreateSecret(t.Context(), "API_KEY", "v")

			requireAPIError(t, err, status)
			assert.NotErrorIs(t, err, ErrAlreadyExists)
		})
	}
}

func TestUpdateSecretNotFound(t *testing.T) {
	s := newRouteServer(t, answerJSON(http.StatusNotFound, `{"error":{"code":2008,"message":"secret not found"}}`))

	err := s.client.UpdateSecret(t.Context(), "API_KEY", "v")

	assert.Equal(t, ErrNotFound, err, "the sentinel comes back bare")
}

func TestUpdateSecretOtherFailuresAreAPIErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusConflict, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newRouteServer(t, answerJSON(status, `{"error":{"code":1,"message":"nope"}}`))

			err := s.client.UpdateSecret(t.Context(), "API_KEY", "v")

			requireAPIError(t, err, status)
			assert.NotErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestDeleteSecretMissingIsSuccess(t *testing.T) {
	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"envelope": answerJSON(http.StatusNotFound, `{"error":{"code":2008,"message":"secret not found"}}`),
		"no body":  answer(http.StatusNotFound, "", ""),
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			s := newRouteServer(t, handler)

			assert.NoError(t, s.client.DeleteSecret(t.Context(), "API_KEY"))
		})
	}

	t.Run("a server error is an API error", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusInternalServerError, `{"error":{"code":1,"message":"boom"}}`))

		err := s.client.DeleteSecret(t.Context(), "API_KEY")

		requireAPIError(t, err, http.StatusInternalServerError)
	})
}

func TestListSecretsEmptyIsNotNil(t *testing.T) {
	for _, reply := range []string{`{"secrets":null}`, `{}`, `{"secrets":[]}`} {
		t.Run(reply, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, reply))

			got, err := s.client.ListSecrets(t.Context())

			require.NoError(t, err)
			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func TestSecretNameIsSegmentChecked(t *testing.T) {
	for _, name := range []string{"", "..", "../x", "a/b"} {
		t.Run(name, func(t *testing.T) {
			s := newRouteServer(t, nil)

			err := s.client.DeleteSecret(t.Context(), name)

			require.Error(t, err)
			assert.Zero(t, s.rec.count())
		})
	}
}
