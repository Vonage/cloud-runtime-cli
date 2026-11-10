package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"sync"
)

// Resolver reaches the bootstrap deployment-api and builds per-region clients.
type Resolver struct {
	core       *core
	base       *url.URL // <bootstrap>/<apiVersion>
	apiVersion string

	mu      sync.Mutex
	clients map[string]*Client // by alias; only lookups that passed every check
}

// NewResolver validates the bootstrap URL (https, or http to a loopback address) and returns a Resolver.
func NewResolver(o Options) (*Resolver, error) { return newResolver(o, nil) }

// newResolver is NewResolver with the TLS configuration that tests use to trust their servers.
func newResolver(o Options, tlsCfg *tls.Config) (*Resolver, error) {
	if o.Bootstrap == "" {
		return nil, errors.New("no deployment API URL to start from")
	}
	c, err := newCore(o, tlsCfg)
	if err != nil {
		return nil, err
	}
	bootstrap, err := parseBootstrap(o.Bootstrap)
	if err != nil {
		return nil, err
	}
	return &Resolver{
		core:       c,
		base:       apiBase(bootstrap, o.APIVersion),
		apiVersion: o.APIVersion,
		clients:    map[string]*Client{},
	}, nil
}

// Regions is GET <bootstrap>/<v>/regions, used by `configure` before any region is known.
func (r *Resolver) Regions(ctx context.Context) ([]Region, error) {
	return r.core.listRegions(ctx, r.base)
}

// Region is GET <bootstrap>/<v>/regions/<alias>. Every URL in the record must pass the URL rule, or it
// fails with "the platform returned an insecure URL for <field>: <url>". A 404 returns an error wrapping ErrNotFound
// (the caller prints v2.6.1's "failed to initialize cli: region does not exist").
func (r *Resolver) Region(ctx context.Context, alias string) (Region, error) {
	c, err := r.clientFor(ctx, alias)
	if err != nil {
		return Region{}, err
	}
	return c.Region(), nil
}

// Client returns the client for a region: its deploymentApiUrl + "/<v>" for API calls and its marketplaceApiUrl
// for template downloads. Records are fetched once per alias per run.
func (r *Resolver) Client(ctx context.Context, alias string) (*Client, error) {
	return r.clientFor(ctx, alias)
}

// clientFor returns the cached client of alias, looking the region up first when there is none.
func (r *Resolver) clientFor(ctx context.Context, alias string) (*Client, error) {
	segment, err := seg(alias)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.clients[alias]; ok {
		return c, nil
	}
	c, err := r.lookup(ctx, segment)
	if err != nil {
		return nil, err
	}
	r.clients[alias] = c
	return c, nil
}

// lookup fetches a region record and checks it. Only when every URL has passed are its deployment and
// marketplace hosts added to the platform host set. The caller holds r.mu.
func (r *Resolver) lookup(ctx context.Context, segment string) (*Client, error) {
	var rec Region
	if err := r.core.read(ctx, routeRegion, endpoint(r.base, "regions", segment), &rec); err != nil {
		return nil, err
	}
	urls, err := r.core.checkRecord(rec)
	if err != nil {
		return nil, err
	}
	r.core.allowHost(urls.deployment)
	if urls.marketplace != nil {
		r.core.allowHost(urls.marketplace)
	}
	return newClient(r.core, apiBase(urls.deployment, r.apiVersion), urls.marketplace, rec), nil
}

// recordURLs are the URLs of a region record that the CLI connects to.
type recordURLs struct {
	deployment  *url.URL
	marketplace *url.URL // nil when the record has none
}

// The names of the URL fields of a region record, as an error shows them.
const (
	fieldDeployment  = "deploymentApiUrl"
	fieldAssets      = "assetsApiUrl"
	fieldMarketplace = "marketplaceApiUrl"
)

// checkRecord applies the URL rule to the URLs of a region record that the platform returned. An empty assets
// or marketplace URL is fine; the deployment URL is required.
func (c *core) checkRecord(rec Region) (recordURLs, error) {
	if rec.DeploymentAPIURL == "" {
		return recordURLs{}, errors.New("the platform returned a region without a " + fieldDeployment)
	}
	deployment, err := c.checkRecordURL(fieldDeployment, rec.DeploymentAPIURL)
	if err != nil {
		return recordURLs{}, err
	}
	if rec.AssetsAPIURL != "" {
		if _, err := c.checkRecordURL(fieldAssets, rec.AssetsAPIURL); err != nil {
			return recordURLs{}, err
		}
	}
	urls := recordURLs{deployment: deployment}
	if rec.MarketplaceAPIURL != "" {
		if urls.marketplace, err = c.checkRecordURL(fieldMarketplace, rec.MarketplaceAPIURL); err != nil {
			return recordURLs{}, err
		}
	}
	return urls, nil
}

// checkRecordURL parses one URL of a record. It must have a host and no userinfo, and be https, or plain http to
// loopback when the bootstrap URL was.
func (c *core) checkRecordURL(field, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || !c.recordSchemeAllowed(u) {
		return nil, fmt.Errorf("the platform returned an insecure URL for %s: %s", field, cutAtRune(dropControls(raw), maxServerIDBytes))
	}
	return u, nil
}

// recordSchemeAllowed is the URL rule narrowed to the schemes that carry requests: a record URL is never a websocket one.
func (c *core) recordSchemeAllowed(u *url.URL) bool {
	if u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS {
		return false
	}
	return c.urlAllowed(u)
}

// apiBase is u with the API version appended when there is one: no trailing slash, query or fragment.
func apiBase(u *url.URL, version string) *url.URL {
	return endpoint(u, url.PathEscape(version))
}
