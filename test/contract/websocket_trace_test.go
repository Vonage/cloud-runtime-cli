//go:build !windows

package contract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	watchHead  = "euw1 WS /packages/<uuid>/build/watch"
	tunnelHead = "other WS /0123456789abcdef0123456789abcdef"
	postHead   = "euw1 POST /packages"
)

// requestBlock is one block of a requests section, as the transcripts have it; more are further lines of the block.
func requestBlock(head, trace string, more ...string) string {
	lines := append([]string{head, "  auth: basic abc123", "  acct: abc123", "  trace: " + trace}, more...)
	return strings.Join(lines, "\n") + "\n"
}

// section joins blocks into a requests section.
func section(blocks ...string) string { return strings.Join(blocks, "\n") }

// compareRelaxed compares two requests sections the way the replay of the binary under test does.
func compareRelaxed(t *testing.T, want, got string) []Failure {
	t.Helper()
	return failuresOf(Compare(mustScenario(t, "version-watch", ""),
		Transcript{Exit: "0", Requests: want}, Result{Exit: "0", Requests: got},
		CompareOptions{RelaxWebsocketTrace: true}), "requests")
}

func TestRelaxWebsocketTraceOnlyTakesTheTraceLineOutOfWebsocketBlocks(t *testing.T) {
	in := section(
		requestBlock(postHead, "yes", "  type: application/json", `  body: {"a":1}`),
		requestBlock(watchHead, "yes"),
		requestBlock(tunnelHead, "no"),
		requestBlock("gh GET /releases/latest", "no"),
		"[peer]\nconnected\n",
	)
	want := section(
		requestBlock(postHead, "yes", "  type: application/json", `  body: {"a":1}`),
		watchHead+"\n  auth: basic abc123\n  acct: abc123\n",
		tunnelHead+"\n  auth: basic abc123\n  acct: abc123\n",
		requestBlock("gh GET /releases/latest", "no"),
		"[peer]\nconnected\n",
	)

	assert.Equal(t, want, RelaxWebsocketTrace(in))
}

func TestRelaxWebsocketTraceLeavesOtherSectionsAlone(t *testing.T) {
	assert.Empty(t, RelaxWebsocketTrace(""))

	plain := section(requestBlock(postHead, "no"), requestBlock("euw1 GET /secrets", "yes"))
	assert.Equal(t, plain, RelaxWebsocketTrace(plain), "no websocket block, nothing changes")

	httpBlock := requestBlock(postHead, "yes", "  body: trace: yes")
	assert.Equal(t, httpBlock, RelaxWebsocketTrace(httpBlock))
}

func TestRelaxWebsocketTraceKeepsEveryOtherLineOfTheBlock(t *testing.T) {
	odd := requestBlock(watchHead, "maybe", "  body: trace: yes", " trace: no", "  trace: no ")

	assert.Equal(t, odd, RelaxWebsocketTrace(odd), "a trace line with another value, or another shape, is not the marker")
}

func TestWebsocketBlockDifferingOnlyInTheTraceMarkerPasses(t *testing.T) {
	for _, head := range []string{watchHead, tunnelHead} {
		for _, sides := range [][2]string{{"no", "yes"}, {"yes", "no"}, {"no", "no"}} {
			want := section(requestBlock(postHead, "yes"), requestBlock(head, sides[0]), requestBlock("euw1 POST /deployments", "yes"))
			got := section(requestBlock(postHead, "yes"), requestBlock(head, sides[1]), requestBlock("euw1 POST /deployments", "yes"))

			assert.Empty(t, compareRelaxed(t, want, got), "%s: %s then %s", head, sides[0], sides[1])
		}
	}
}

func TestWebsocketBlockDifferingInAnythingElseStillFails(t *testing.T) {
	base := requestBlock(watchHead, "no")
	tests := []struct {
		name      string
		want, got string
	}{
		{"credentials", base, watchHead + "\n  auth: none\n  acct: abc123\n  trace: yes\n"},
		{"account", base, watchHead + "\n  auth: basic abc123\n  acct: other\n  trace: yes\n"},
		{"a further line", base, requestBlock(watchHead, "yes", "  type: text/plain")},
		{"the target", base, requestBlock("euw1 WS /packages/<uuid>/build/other", "yes")},
		{"the region", base, requestBlock("use1 WS /packages/<uuid>/build/watch", "yes")},
		{"the lane", base, requestBlock(tunnelHead, "yes")},
		{"a missing block", base, ""},
		{"an extra block", base, section(requestBlock(watchHead, "yes"), requestBlock(watchHead, "yes"))},
		{
			"the order of blocks",
			section(requestBlock(watchHead, "no"), requestBlock(postHead, "yes")),
			section(requestBlock(postHead, "yes"), requestBlock(watchHead, "yes")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := compareRelaxed(t, tt.want, tt.got)

			require.Len(t, fs, 1)
			assert.Contains(t, fs[0].Detail, "requests differs from the transcript")
		})
	}
}

func TestTraceMarkerOfAnyOtherBlockIsStillCompared(t *testing.T) {
	tests := []struct{ name, head string }{
		{"an HTTP block of a region", postHead},
		{"a read", "euw1 GET /secrets"},
		{"the release lane", "gh GET /releases/latest"},
		{"a call on the other listener", "other GET /public"},
		{"a block without a lane", "- GET /x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := section(requestBlock(watchHead, "no"), requestBlock(tt.head, "no"))
			got := section(requestBlock(watchHead, "yes"), requestBlock(tt.head, "yes"))

			fs := compareRelaxed(t, want, got)

			require.Len(t, fs, 1)
			assert.Contains(t, fs[0].Detail, tt.head)
		})
	}
}

func TestTraceMarkerOfAWebsocketBlockIsComparedWhenTheOptionIsOff(t *testing.T) {
	want, got := requestBlock(watchHead, "no"), requestBlock(watchHead, "yes")
	sc := mustScenario(t, "version-watch", "")

	fs := failuresOf(Compare(sc, Transcript{Exit: "0", Requests: want}, Result{Exit: "0", Requests: got}, CompareOptions{}), "requests")

	require.Len(t, fs, 1, "a replay of the baseline itself compares every line")
	assert.Contains(t, fs[0].Detail, "trace: no")
}
