//go:build !windows

package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tunnelBody is a scenario whose stand-in CLI dials the debug host's websocket until the peer closes it, with trace
// sent as the trace id of the handshake when it is not empty.
func tunnelBody(trace string) string {
	args := `tunnel, "ws://${OTHER_HOST}/0123456789abcdef0123456789abcdef"`
	if trace != "" {
		args += ", " + trace
	}
	return "args: [" + args + "]\nexpect-reads: []\nsteps:\n  - {wait-peer: connected}\n  - {peer-close: {code: 1000, text: done}}\n"
}

// The credential lines of gh blocks are pinned the same way by TestGitHubCredentialLinesAreIgnoredOnlyWhileRecordedFromACommit.
func TestWebsocketTraceIsIgnoredOnlyWhenTheBinaryUnderTestIsReplayed(t *testing.T) {
	labels := map[string]string{
		"recorded from a commit":      "f4b9df0 0123456789abcdef0123456789abcdef01234567",
		"recorded from a release tag": stubLabel,
	}
	for name, label := range labels {
		t.Run(name, func(t *testing.T) {
			c := stubConfig(t)
			c.BaselineLabel = label
			putScenario(t, c, "debug-trace", tunnelBody(""))
			recordAll(t, c)
			sends := putScenario(t, c, "debug-trace", tunnelBody("t-1"))

			c.NoGraphQL = true
			assert.Empty(t, Check(t, c, sends), "the binary under test dials with a trace id, as the baseline did not")

			c.NoGraphQL = false
			got := failuresOf(Check(t, c, sends), "requests")
			require.Len(t, got, 1, "a replay of the baseline compares every line")
			assert.Contains(t, got[0].Detail, "trace: ")
		})
	}
}
