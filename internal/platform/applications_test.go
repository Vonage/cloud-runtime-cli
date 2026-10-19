package platform

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationRoutesWireFormat(t *testing.T) {
	const (
		created = `{"applicationId":"app-1","applicationName":"my app"}`
		listed  = `{"applications":[{"id":"app-1","name":"my app"},{"id":"app-2","name":"other"}]}`
	)
	tests := []struct {
		name       string
		reply      string
		call       func(ctx context.Context, c *Client) (any, error)
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
		want       any
	}{
		{
			name:  "create",
			reply: created,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CreateApplication(ctx, "my app", false, true, true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v0.3/applications",
			wantBody:   `{"name":"my app","enableRtc":false,"enableVoice":true,"enableMessages":true}`,
			want:       CreatedApplication{ApplicationID: "app-1", ApplicationName: "my app"},
		},
		{
			name:  "create with every capability",
			reply: created,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CreateApplication(ctx, "x", true, true, true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v0.3/applications",
			wantBody:   `{"name":"x","enableRtc":true,"enableVoice":true,"enableMessages":true}`,
			want:       CreatedApplication{ApplicationID: "app-1", ApplicationName: "my app"},
		},
		{
			name:  "list without a filter still sends the pattern",
			reply: listed,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ListApplications(ctx, "")
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v0.3/applications",
			wantQuery:  "pattern=",
			want:       []Application{{ID: "app-1", Name: "my app"}, {ID: "app-2", Name: "other"}},
		},
		{
			name:  "list escapes the filter",
			reply: listed,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ListApplications(ctx, "a b&c")
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v0.3/applications",
			wantQuery:  "pattern=a+b%26c",
			want:       []Application{{ID: "app-1", Name: "my app"}, {ID: "app-2", Name: "other"}},
		},
		{
			name:  "generate keys",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.GenerateApplicationKeys(ctx, "app-1")
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/v0.3/applications/app-1/keys",
		},
		{
			name:  "generate keys escapes the id",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.GenerateApplicationKeys(ctx, "an id")
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/v0.3/applications/an%20id/keys",
		},
		{
			name:  "delete",
			reply: `{}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.DeleteApplication(ctx, "app-1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v0.3/applications/app-1",
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
			assert.Equal(t, tc.wantQuery, req.RawQuery)
			assert.Equal(t, tc.wantBody, string(req.Body))
			if tc.wantBody == "" {
				assert.Empty(t, req.Header.Get("Content-Type"))
			} else {
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			}
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestListApplicationsWithNoneIsAnEmptyList(t *testing.T) {
	for _, reply := range []string{`{"applications":null}`, `{}`, `{"applications":[]}`} {
		t.Run(reply, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, reply))

			got, err := s.client.ListApplications(t.Context(), "")

			require.NoError(t, err)
			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func TestCreateApplicationFailureIsAnAPIError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newRouteServer(t, answerJSON(status, `{"error":{"code":4001,"message":"nope"}}`))

			_, err := s.client.CreateApplication(t.Context(), "my app", true, true, true)

			apiErr := apiErrorWithStatus(t, err, status)
			assert.Equal(t, 4001, apiErr.ServerCode)
			assert.Equal(t, "nope", apiErr.Message)
			assert.NotErrorIs(t, err, ErrNotFound)
			assert.NotErrorIs(t, err, ErrAlreadyExists)
			assert.Equal(t, 1, s.rec.count())
		})
	}
}

func TestDeleteApplicationNotFound(t *testing.T) {
	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"envelope":   answerJSON(http.StatusNotFound, `{"error":{"code":2007,"message":"application not found"}}`),
		"plain text": answer(http.StatusNotFound, "text/plain", "no such application"),
		"no body":    answer(http.StatusNotFound, "", ""),
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			s := newRouteServer(t, handler)

			err := s.client.DeleteApplication(t.Context(), "app-1")

			assert.Equal(t, ErrNotFound, err, "the sentinel comes back bare")
		})
	}

	t.Run("a server error is an API error", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusInternalServerError, `{"error":{"code":1,"message":"boom"}}`))

		err := s.client.DeleteApplication(t.Context(), "app-1")

		requireAPIError(t, err, http.StatusInternalServerError)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestApplicationIDsAreSegmentChecked(t *testing.T) {
	calls := map[string]func(ctx context.Context, c *Client, id string) error{
		"GenerateApplicationKeys": func(ctx context.Context, c *Client, id string) error {
			return c.GenerateApplicationKeys(ctx, id)
		},
		"DeleteApplication": func(ctx context.Context, c *Client, id string) error {
			return c.DeleteApplication(ctx, id)
		},
	}
	for name, call := range calls {
		for _, id := range []string{"", "..", "a/b"} {
			t.Run(name+" "+id, func(t *testing.T) {
				s := newRouteServer(t, nil)

				err := call(t.Context(), s.client, id)

				require.Error(t, err)
				assert.Zero(t, s.rec.count())
			})
		}
	}
}
