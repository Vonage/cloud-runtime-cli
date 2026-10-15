package endpoint

import (
	"fmt"
	"regexp"
)

var regionPattern = regexp.MustCompile(`^aws\.[a-z0-9]+$`)

// ValidateRegion returns `invalid --region "<value>": expected aws.<name>` unless alias matches ^aws\.[a-z0-9]+$
// An empty alias is valid here (it means "not given").
func ValidateRegion(alias string) error {
	if alias == "" || regionPattern.MatchString(alias) {
		return nil
	}
	return fmt.Errorf("invalid --region %q: expected aws.<name>", alias)
}
