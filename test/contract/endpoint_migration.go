//go:build !windows

package contract

import (
	"regexp"
	"strings"
)

// The binary under test moves users off the GraphQL settings: its help lists --endpoint above a deprecated
// --graphql-endpoint, and a GraphQL setting prints one deprecation line on stderr. These are the only differences
// that relaxEndpointMigration takes out, so the rest of each field is still compared with the baseline.
const (
	baselineHelpLine       = "      --graphql-endpoint string   Graphql endpoint used to fetch metadata\n"
	baselineCompletionLine = "--graphql-endpoint\tGraphql endpoint used to fetch metadata\n"
	endpointUsage          = "Platform API base URL (overrides --graphql-endpoint and the config file)"
	deprecatedUsage        = "Deprecated: use --endpoint. GraphQL endpoint used to fetch metadata"
)

var (
	currentHelpLines = "      --endpoint string           " + endpointUsage + "\n" +
		"      --graphql-endpoint string   " + deprecatedUsage + "\n"
	currentCompletionLines = "--endpoint\t" + endpointUsage + "\n--graphql-endpoint\t" + deprecatedUsage + "\n"

	deprecationNotice = regexp.MustCompile(`(?m)^! (--graphql-endpoint|graphql_endpoint in \S+) is deprecated[^\n]*\n`)
)

// relaxEndpointMigration returns the baseline as the binary under test is meant to differ from it, and the result
// without its deprecation lines.
func relaxEndpointMigration(base Transcript, got Result) (Transcript, Result) {
	base.Stdout = strings.ReplaceAll(base.Stdout, baselineHelpLine, currentHelpLines)
	base.Stdout = strings.ReplaceAll(base.Stdout, baselineCompletionLine, currentCompletionLines)
	got.Stderr = deprecationNotice.ReplaceAllString(got.Stderr, "")
	return base, got
}
