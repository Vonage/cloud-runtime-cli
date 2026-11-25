package debugsession

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlerRelaysPlatformServiceCallByServiceAndPath(t *testing.T) {
	caller := &fakeCaller{}
	body := `{"id":"1"}`
	req := newHandlerRequest(t, http.MethodPut, "/_vcr/data-api/v1/items?collection=orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	callHandler(t, caller, req)

	got := caller.only(t)
	assert.True(t, got.BrokerAuth)
	assert.Equal(t, "data-api", got.FAASFunction)
	assert.Equal(t, "/v1/items", got.Path)
	assert.Empty(t, got.URL)
	assert.Equal(t, http.MethodPut, got.Method)
	assert.Equal(t, url.Values{"collection": {"orders"}}, got.Query)
	assert.Equal(t, []byte(body), got.Payload)
	assert.Equal(t, map[string]string{"Content-Type": "application/json"}, got.Headers)
}

func TestHandlerPlatformServicePath(t *testing.T) {
	tests := []struct{ target, wantPath string }{
		{"/_vcr/data-api", "/"},
		{"/_vcr/data-api/v1/collections/a%2Fb", "/v1/collections/a%2Fb"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			caller := &fakeCaller{}

			callHandler(t, caller, newHandlerRequest(t, http.MethodGet, tt.target, nil))

			got := caller.only(t)
			assert.Equal(t, "data-api", got.FAASFunction)
			assert.Equal(t, tt.wantPath, got.Path)
		})
	}
}

func TestHandlerRefusesAPlatformServiceThatIsNotADNSLabel(t *testing.T) {
	for _, target := range []string{
		"/_vcr/", "/_vcr//x", "/_vcr/Data-API/x", "/_vcr/-x/y", "/_vcr/data-api.example.com/x",
		"/_vcr/x-/y", "/_vcr/" + strings.Repeat("a", 64) + "/x",
	} {
		t.Run(target, func(t *testing.T) {
			caller := &fakeCaller{}

			rec := callHandler(t, caller, newHandlerRequest(t, http.MethodGet, target, nil))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, caller.requests())
		})
	}
}

func TestHandlerAcceptsAPlatformServiceOf63Characters(t *testing.T) {
	caller := &fakeCaller{}
	service := "a" + strings.Repeat("-", 61) + "1"

	callHandler(t, caller, newHandlerRequest(t, http.MethodGet, "/_vcr/"+service+"/x", nil))

	assert.Equal(t, service, caller.only(t).FAASFunction)
}
