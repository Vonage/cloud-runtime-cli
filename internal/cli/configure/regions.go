package configure

import (
	"fmt"
	"regexp"
	"strings"

	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	// defaultGraphqlEndpoint is written when nothing says otherwise.
	defaultGraphqlEndpoint = "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"
	graphqlEndpointFormat  = "https://graphql.%s.runtime.vonage.cloud/v1/graphql"
	regionAliasPrefix      = "aws."
)

// testRegion matches the regions the picker leaves out: the ones whose name or alias says "test".
var testRegion = regexp.MustCompile("(?i)test")

// choices is what the region picker offers.
type choices struct {
	labels  []string          // in the order the platform listed them
	aliasOf map[string]string // label to the alias exactly as the platform sent it
	labelOf map[string]string // that alias to its label
}

// regionChoices makes the picker of the regions, without the test regions. The name and the alias are cleaned of
// terminal control sequences in the label only: the alias that is written to the file stays exact.
func regionChoices(regions []platform.Region) choices {
	c := choices{aliasOf: map[string]string{}, labelOf: map[string]string{}}
	for _, r := range regions {
		if testRegion.MatchString(r.Name) || testRegion.MatchString(r.Alias) {
			continue
		}
		label := fmt.Sprintf("%s - (%s)", term.Sanitize(r.Name), term.Sanitize(r.Alias))
		c.labels = append(c.labels, label)
		c.aliasOf[label] = r.Alias
		c.labelOf[r.Alias] = label
	}
	return c
}

// graphqlEndpointToWrite is the graphql_endpoint of the new file: the --graphql-endpoint flag, else the endpoint
// of the --region flag, else the one already in the file, else the default. The key is written because a user who
// goes back to an older vcr needs it.
func graphqlEndpointToWrite(flagEndpoint, flagRegion, stored string) string {
	switch {
	case flagEndpoint != "":
		return flagEndpoint
	case flagRegion != "":
		return fmt.Sprintf(graphqlEndpointFormat, strings.TrimPrefix(flagRegion, regionAliasPrefix))
	case stored != "":
		return stored
	default:
		return defaultGraphqlEndpoint
	}
}
