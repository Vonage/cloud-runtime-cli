package platform

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// uploadBackoff are the delays before the retries. The upload makes one attempt more than it has delays.
var uploadBackoff = [...]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// retryJitter is how far a delay may stray from its base, as a fraction of it.
const retryJitter = 0.2

// locationFailure starts the error that net/http returns for a redirect answer whose Location it cannot parse.
const locationFailure = "failed to parse Location header"

// retryReason says whether a failed attempt may be repeated and how the notice names the failure. The answers of a
// gateway (502, 503, 504) are worth another attempt, and so is a failure of the network. A network failure is not
// retried once the context has ended, nor when policy refused the request (a dial or a redirect), nor when net/http
// could not follow a redirect: another attempt would meet the same.
func retryReason(ctx context.Context, err error) (reason string, again bool) {
	var apiErr *clierr.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatus {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "HTTP " + strconv.Itoa(apiErr.HTTPStatus), true
		}
		return "", false
	}

	if ctxEnded(ctx) != nil || !IsTransportError(err) {
		return "", false
	}
	var urlErr *url.Error
	errors.As(err, &urlErr)
	return term.Sanitize(urlErr.Err.Error()), true
}

// IsTransportError reports a failure of the network that another attempt may get past: a *url.Error from Go's HTTP
// client that is neither a policy refusal (a dial or a redirect) nor a redirect net/http could not follow.
func IsTransportError(err error) bool {
	var urlErr *url.Error
	var refusal *policyError
	return errors.As(err, &urlErr) && !errors.As(err, &refusal) && !isLocationFailure(urlErr)
}

// isLocationFailure recognises the one redirect failure that net/http reports before the redirect rule runs. It
// has no type of its own, so only its text tells it apart.
func isLocationFailure(err *url.Error) bool {
	return err.Err != nil && strings.HasPrefix(err.Err.Error(), locationFailure)
}

// waitToRetry announces retry number retry+1 and waits out its delay. It returns the context's error when the
// context ends first; once the context has ended it announces nothing.
func (c *Client) waitToRetry(ctx context.Context, retry int, reason string) error {
	if err := ctxEnded(ctx); err != nil {
		return err
	}
	base := uploadBackoff[retry]
	fmt.Fprintf(c.core.notices, "! upload failed (%s); retrying in %s (retry %d of %d)\n", reason, base, retry+1, len(uploadBackoff))
	return c.core.clock.Sleep(ctx, jittered(base))
}

// jittered spreads base uniformly over [0.8, 1.2) of itself, so that clients that failed together do not retry together.
func jittered(base time.Duration) time.Duration {
	factor := 1 - retryJitter + 2*retryJitter*rand.Float64() //nolint:gosec // jitter is not a secret
	return time.Duration(float64(base) * factor)
}
