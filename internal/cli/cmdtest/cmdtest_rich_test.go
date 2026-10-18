package cmdtest_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/term"
)

func TestHarnessRichModeBuildsARichIO(t *testing.T) {
	srv := startFake(t)

	classic := cmdtest.New(t, srv, cmdtest.Options{})
	rich := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})
	static := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Static})

	assert.Equal(t, term.Classic, classic.Env.IO.Mode())
	assert.Equal(t, term.Rich, rich.Env.IO.Mode())
	assert.Equal(t, term.Static, static.Env.IO.Mode())
	require.NotNil(t, rich.Env.Client, "the setup still resolves the region")
	assert.Empty(t, rich.Requests())
}

func TestHarnessScreenAndRaw(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	var bold string
	err := h.Run(t.Context(), probe(func(_ *cobra.Command, env *cmdenv.Env) error {
		bold = env.IO.Style().Bold("warn")
		fmt.Fprint(env.IO.Out, "listing\r\x1b[2Kdone\n")
		fmt.Fprint(env.IO.ErrOut, bold+"\n")
		return nil
	}))

	require.NoError(t, err)
	assert.Equal(t, "done\n", h.Screen(), "an erased line is rewritten, as on a terminal")
	assert.Equal(t, "warn\n", h.ErrScreen(), "colour leaves no trace on the screen")
	assert.Equal(t, bold+"\n", h.ErrRaw(), "the raw bytes keep it")
	assert.NotEqual(t, "warn\n", h.ErrRaw())
}

func TestHarnessScreenReadsTheBuffersInClassicMode(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), probe(func(_ *cobra.Command, env *cmdenv.Env) error {
		fmt.Fprint(env.IO.Out, "out\n")
		fmt.Fprint(env.IO.ErrOut, "err\n")
		return nil
	}))

	require.NoError(t, err)
	assert.Equal(t, "out\n", h.Screen())
	assert.Equal(t, "err\n", h.ErrScreen())
	assert.Equal(t, "err\n", h.ErrRaw())
	assert.Equal(t, h.Stdout.String(), h.Screen())
}

func TestHarnessRichModeCanPrompt(t *testing.T) {
	srv := startFake(t)
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			script := &cmdtest.Script{YesNos: []bool{true}}
			h := cmdtest.New(t, srv, cmdtest.Options{Mode: mode, Prompt: script})

			ok, err := h.Env.Prompt.YesNo("Proceed?")

			assert.True(t, h.Env.IO.CanPrompt())
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: "Proceed?"}}, script.Asked)
		})
	}

	t.Run("classic keeps its own rule", func(t *testing.T) {
		assert.False(t, cmdtest.New(t, srv, cmdtest.Options{}).Env.IO.CanPrompt())
		assert.True(t, cmdtest.New(t, srv, cmdtest.Options{TTY: true}).Env.IO.CanPrompt())
	})
}

func TestHarnessRichModeReadsStdin(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Static, Stdin: "typed\n"})

	data := make([]byte, len("typed\n"))
	n, err := h.Env.IO.In.Read(data)

	require.NoError(t, err)
	assert.Equal(t, "typed\n", string(data[:n]))
}

func TestHarnessRichModeDrivesTheClockOfTheIO(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	step := h.Env.IO.StartStep("Working")
	require.NoError(t, h.Clock.Sleep(t.Context(), 2500*time.Millisecond))
	step.OK("Worked")

	assert.Contains(t, h.Screen(), "Worked  (2.5s)", "the step measured its time on the harness clock")
	assert.Equal(t, "✓ Worked\n", cmdtest.Collapse(h.Screen()), "the duration token is not part of the comparison")
}

func TestHarnessWidth(t *testing.T) {
	srv := startFake(t)
	name := strings.Repeat("n", 120)
	tests := []struct {
		name  string
		width int
		want  int // the width of the table's row: cut to the terminal, or whole when it fits
	}{
		{"the default is 100 columns", 0, 100},
		{"a narrow terminal", 50, 50},
		{"a wide terminal cuts nothing", 150, 4 + 2 + 120},
	}
	for modeName, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		for _, tt := range tests {
			t.Run(modeName+" "+tt.name, func(t *testing.T) {
				h := cmdtest.New(t, srv, cmdtest.Options{Mode: mode, Width: tt.width})

				h.Env.IO.RichTable([]string{"ID", "Name"}, [][]string{{"id-1", name}}, []int{0}, "item", "None.")

				row := strings.Split(h.Screen(), "\n")[1]
				assert.Equal(t, tt.want, utf8.RuneCountInString(row), row)
			})
		}
	}
}

func TestHarnessWidthDoesNotChangeAClassicRun(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{Width: 30})

	assert.Equal(t, term.Classic, h.Env.IO.Mode())
	assert.NotNil(t, h.Stdout)
}

func TestCollapse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"a duration token goes", "✓ Source uploaded  (2.1s)", "✓ Source uploaded"},
		{"the token goes before the spaces collapse", "a  (2.1s)   b", "a b"},
		{"minutes and seconds", "x  (1m04s)", "x"},
		{"milliseconds", "x  (12ms)", "x"},
		{"runs of spaces collapse to one", "a     b    c", "a b c"},
		{"a count in parentheses stays", "✓ Files packaged  (2 files)", "✓ Files packaged (2 files)"},
		{"lines stay lines", "a  b\n  c", "a b\n c"},
		{"one space stays", "a b", "a b"},
		{"no token without its two spaces", "a (2.1s)", "a (2.1s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cmdtest.Collapse(tt.in))
		})
	}
}
