package platform

import "net/http"

// checkRedirect is the http.Client's redirect policy. It runs for GET and HEAD only in practice: any other
// original method is refused here, or by send when Go hands back a 307 or 308 it cannot replay.
func (c *core) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return policyErrorf("stopped after %d redirects", maxRedirects)
	}
	// Go has already turned a POST or DELETE into a GET on 301, 302 and 303, so the original request decides.
	if method := via[0].Method; method != http.MethodGet && method != http.MethodHead {
		return policyErrorf("unexpected redirect from %s", via[len(via)-1].URL.Host)
	}
	if !c.urlAllowed(req.URL) {
		return policyErrorf("refusing redirect to %s: only https is allowed", req.URL.Host)
	}
	return nil
}
