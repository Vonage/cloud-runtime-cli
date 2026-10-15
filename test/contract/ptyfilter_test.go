//go:build !windows

package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const sampleWarning = "2026/10/15 19:22:52.025235 WARN RESTY Using Basic Auth in HTTP mode is not secure, use HTTPS\r\n"

// filterAll feeds the chunks to a new filter and returns everything it showed.
func filterAll(chunks ...string) string {
	f := &noiseFilter{}
	var out []byte
	for _, c := range chunks {
		out = append(out, f.filter([]byte(c))...)
	}
	return string(append(out, f.flush()...))
}

func TestNoiseFilterDropsWarningLines(t *testing.T) {
	assert.Equal(t, "before after", filterAll("before "+sampleWarning+"after"))
	assert.Equal(t, "ab", filterAll("a"+sampleWarning+sampleWarning+"b"))
	assert.Equal(t, "x\nnext\n", filterAll("x\n2026/10/15 19:22:52 WARN RESTY short\nnext\n"), "a line feed alone ends a line")
	assert.Equal(t, "⠋ Retrieving... ", filterAll("⠋ Retrieving... "+sampleWarning), "a warning that follows a spinner frame leaves the frame")
}

func TestNoiseFilterKeepsEverythingElse(t *testing.T) {
	for _, text := range []string{
		"? Enter your Vonage api key: (abc123) ",
		"2026/10/15 19:22:52 an ordinary log line\r\n",
		"2026/10/15 19:22:52.5 WARN something else\r\n",
		"WARN RESTY without a time\r\n",
		"12 34 2026",
		"2026/10/15 ",
		"\x1b[999;999f\x1b[6n",
		"",
	} {
		assert.Equal(t, text, filterAll(text), "%q", text)
		assert.Equal(t, text, filterAll(text[:len(text)/2], text[len(text)/2:]), "split: %q", text)
	}
}

func TestNoiseFilterWorksWhereverTheChunksSplit(t *testing.T) {
	text := "prompt: " + sampleWarning + "next"
	for i := 0; i <= len(text); i++ {
		assert.Equal(t, "prompt: next", filterAll(text[:i], text[i:]), "split at %d", i)
	}
	for i := 0; i < len(text); i += 7 {
		var chunks []string
		for j := 0; j < len(text); j += 7 {
			chunks = append(chunks, text[j:min(j+7, len(text))])
		}
		assert.Equal(t, "prompt: next", filterAll(chunks...), "chunks of 7 bytes")
	}
}

func TestNoiseFilterHoldsAStartedLineUntilItIsComplete(t *testing.T) {
	f := &noiseFilter{}
	assert.Equal(t, "ok ", string(f.filter([]byte("ok 2026/10/15 19:22:52.025235 WARN RE"))), "the started line is held, the text before it shown")
	assert.Equal(t, "", string(f.filter([]byte("STY Using Basic Auth"))))
	assert.Equal(t, "done", string(f.filter([]byte(" in HTTP mode\r\ndone"))))
	assert.Empty(t, f.flush())

	g := &noiseFilter{}
	assert.Equal(t, "", string(g.filter([]byte("2026/10/15 19:22:52 WARN RESTY never finished"))))
	assert.Equal(t, "2026/10/15 19:22:52 WARN RESTY never finished", string(g.flush()), "the end of the stream releases what is held")
}
