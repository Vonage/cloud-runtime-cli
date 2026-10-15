package platform

import (
	"errors"
	"io"

	"vonage-cloud-runtime-cli/internal/clock"
)

// ErrNotFound is returned (wrapped) when a read route answers 404 with the error envelope.
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists is returned (wrapped) when CreateSecret is answered 409.
var ErrAlreadyExists = errors.New("already exists")

// Credentials are sent as HTTP Basic api_key:api_secret plus X-Neru-ApiAccountId: <api_key>,
// only to platform hosts over https (or loopback under the loopback rule).
type Credentials struct {
	APIKey    string
	APISecret string
}

// Options configure a Resolver.
type Options struct {
	Bootstrap  string // from endpoint.Bootstrap, without the version segment
	APIVersion string // "v0.3" in releases
	Creds      Credentials
	UserAgent  string      // buildinfo.Info.UserAgent()
	Clock      clock.Clock // retry and reconnect delays
	Notices    io.Writer   // stderr, for the upload-retry notice
}
