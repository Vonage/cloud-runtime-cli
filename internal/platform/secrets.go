package platform

import (
	"context"
	"net/http"
)

// secretsBody is the request of CreateSecret and UpdateSecret: a list of one secret.
type secretsBody struct {
	Secrets []secretEntry `json:"secrets"`
}

type secretEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func oneSecret(name, value string) secretsBody {
	return secretsBody{Secrets: []secretEntry{{Name: name, Value: value}}}
}

func (c *Client) CreateSecret(ctx context.Context, name, value string) error {
	err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "secrets"), oneSecret(name, value), nil)
	if hasStatus(err, http.StatusConflict) {
		return ErrAlreadyExists
	}
	return err
} // POST /secrets

func (c *Client) UpdateSecret(ctx context.Context, name, value string) error {
	err := c.core.sendJSON(ctx, http.MethodPatch, endpoint(c.base, "secrets"), oneSecret(name, value), nil)
	if hasStatus(err, http.StatusNotFound) {
		return ErrNotFound
	}
	return err
} // PATCH /secrets

func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	segment, err := seg(name)
	if err != nil {
		return err
	}
	err = c.core.sendJSON(ctx, http.MethodDelete, endpoint(c.base, "secrets", segment), nil, nil)
	if hasStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
} // DELETE /secrets/<name>; 404 = success

func (c *Client) ListSecrets(ctx context.Context) ([]string, error) {
	var out struct {
		Secrets []string `json:"secrets"`
	}
	if err := c.core.getJSON(ctx, endpoint(c.base, "secrets"), &out); err != nil {
		return nil, err
	}
	return nonNil(out.Secrets), nil
} // GET /secrets
