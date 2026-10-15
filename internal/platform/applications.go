package platform

import (
	"context"
	"net/http"
	"net/url"
)

type CreatedApplication struct {
	ApplicationID   string `json:"applicationId"`
	ApplicationName string `json:"applicationName"`
}

type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createApplicationBody is the request of CreateApplication; the order of its fields is the order of its JSON keys.
type createApplicationBody struct {
	Name           string `json:"name"`
	EnableRTC      bool   `json:"enableRtc"`
	EnableVoice    bool   `json:"enableVoice"`
	EnableMessages bool   `json:"enableMessages"`
}

func (c *Client) CreateApplication(ctx context.Context, name string, rtc, voice, messages bool) (CreatedApplication, error) {
	body := createApplicationBody{Name: name, EnableRTC: rtc, EnableVoice: voice, EnableMessages: messages}
	var out CreatedApplication
	if err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "applications"), body, &out); err != nil {
		return CreatedApplication{}, err
	}
	return out, nil
} // POST /applications

func (c *Client) ListApplications(ctx context.Context, filter string) ([]Application, error) {
	u := endpoint(c.base, "applications")
	u.RawQuery = url.Values{"pattern": {filter}}.Encode()
	var out struct {
		Applications []Application `json:"applications"`
	}
	if err := c.core.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	return nonNil(out.Applications), nil
} // GET /applications?pattern=<filter>

func (c *Client) GenerateApplicationKeys(ctx context.Context, appID string) error {
	id, err := seg(appID)
	if err != nil {
		return err
	}
	return c.core.sendJSON(ctx, http.MethodPatch, endpoint(c.base, "applications", id, "keys"), nil, nil)
}

func (c *Client) DeleteApplication(ctx context.Context, appID string) error {
	id, err := seg(appID)
	if err != nil {
		return err
	}
	err = c.core.sendJSON(ctx, http.MethodDelete, endpoint(c.base, "applications", id), nil, nil)
	if hasStatus(err, http.StatusNotFound) {
		return ErrNotFound
	}
	return err
}
