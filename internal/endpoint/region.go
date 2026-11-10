package endpoint

import (
	"fmt"
	"slices"
	"strings"
)

// supportedRegions are the region aliases clients may use, sorted. Standby regions are deliberately absent.
var supportedRegions = []string{"aws.apse1", "aws.apse2", "aws.euw1", "aws.use1"}

// ValidateRegion returns `invalid --region "<value>": expected one of <supported regions>` unless alias is one of
// the supported regions. An empty alias is valid here (it means "not given").
func ValidateRegion(alias string) error {
	if alias == "" || slices.Contains(supportedRegions, alias) {
		return nil
	}
	return fmt.Errorf("invalid --region %q: expected one of %s", alias, strings.Join(supportedRegions, ", "))
}
