package platform

import (
	"errors"

	"vonage-cloud-runtime-cli/internal/clierr"
)

// hasStatus reports whether err is an API error with the given HTTP status.
func hasStatus(err error, status int) bool {
	var apiErr *clierr.APIError
	return errors.As(err, &apiErr) && apiErr.HTTPStatus == status
}
