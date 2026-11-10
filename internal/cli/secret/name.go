package secret

import (
	"fmt"
	"regexp"
)

// nameFormat is the shape of a secret name: a valid environment variable name.
const nameFormat = "^[a-zA-Z_]+[a-zA-Z0-9_]*$"

var namePattern = regexp.MustCompile(nameFormat)

// validateName checks the name of a secret that create and update are about to send.
func validateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid secret name: must follow the regex format %q", nameFormat)
	}
	return nil
}
