package platform

import "net/url"

// Region is the region record.
type Region struct {
	Name              string `json:"name"`
	Alias             string `json:"alias"`
	Default           bool   `json:"default"`
	DeploymentAPIURL  string `json:"deploymentApiUrl"`
	AssetsAPIURL      string `json:"assetsApiUrl"`
	MarketplaceAPIURL string `json:"marketplaceApiUrl"`
	EndpointURLScheme string `json:"endpointUrlScheme"`
	DebuggerURLScheme string `json:"debuggerUrlScheme"`
	HostTemplate      string `json:"hostTemplate"`
}

// Client calls one region's deployment-api.
type Client struct {
	core        *core
	base        *url.URL // <deploymentApiUrl>/<apiVersion>
	marketplace *url.URL // <marketplaceApiUrl>; nil when the region has none
	region      Region
}

func newClient(c *core, base, marketplace *url.URL, region Region) *Client {
	return &Client{core: c, base: base, marketplace: marketplace, region: region}
}

// Region returns the record this client was built from.
func (c *Client) Region() Region { return c.region }
