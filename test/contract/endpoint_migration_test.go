//go:build !windows

package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelaxEndpointMigration(t *testing.T) {
	base := Transcript{
		Stdout: "INHERITED FLAGS\n" + baselineHelpLine + "      --help   Show help\n--api-key\tk\n" + baselineCompletionLine,
		Stderr: "X region does not exist\n",
	}
	got := Result{
		Stdout: "INHERITED FLAGS\n" + currentHelpLines + "      --help   Show help\n--api-key\tk\n" + currentCompletionLines,
		Stderr: "! --graphql-endpoint is deprecated; use --endpoint ${FAKE}/api/deployment instead\n" +
			"! graphql_endpoint in ~/.vcr-cli is deprecated; add \"endpoint = x\" to the file, or run vcr configure\n" +
			"X region does not exist\n",
	}

	b, g := relaxEndpointMigration(base, got)

	assert.Equal(t, g.Stdout, b.Stdout)
	assert.Equal(t, b.Stderr, g.Stderr)
}

func TestRelaxEndpointMigrationKeepsOtherDifferences(t *testing.T) {
	base := Transcript{Stdout: "a\n", Stderr: "X region does not exist\n"}
	got := Result{Stdout: "b\n", Stderr: "! --graphql-endpoint is deprecated; use --endpoint u instead\nX another error\n"}

	failures := Compare(Scenario{ID: "s"}, base, got, CompareOptions{NoGraphQL: true})

	fields := map[string]bool{}
	for _, f := range failures {
		fields[f.Field] = true
	}
	assert.True(t, fields["stdout"])
	assert.True(t, fields["stderr"], "a changed error is still caught behind a notice")
}
