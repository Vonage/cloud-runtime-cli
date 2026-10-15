package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientHoldsTheRecordAndURLs(t *testing.T) {
	srv := startPlain(t, newRecorder(nil))
	c, _, _ := newTestClient(t, srv)

	require.NotNil(t, c.core)
	assert.Equal(t, srv.URL+"/v0.3", c.base.String())
	assert.Nil(t, c.marketplace)
	assert.Equal(t, "aws.euw1", c.Region().Alias)
	assert.Equal(t, srv.URL, c.Region().DeploymentAPIURL)
}

func TestNewClientSharesItsCore(t *testing.T) {
	shared := newTestCore(t, "https://api.example.test", nil)
	base := mustURL(t, "https://api.example.test/v0.3")
	market := mustURL(t, "https://market.example.test")

	first := newClient(shared, base, market, Region{Alias: "aws.euw1"})
	second := newClient(shared, base, nil, Region{Alias: "aws.use1"})

	assert.Same(t, first.core, second.core)
	assert.Same(t, market, first.marketplace)
	assert.Equal(t, "aws.use1", second.Region().Alias)
}
