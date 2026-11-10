//go:build !windows

package contract

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectAnyDecodes(t *testing.T) {
	sc, err := ParseScenario("version-any", []byte("description: any text\nargs: []\nsteps:\n"+
		"  - {expect-any: [\"first text\", \"second text\"], timeout: 5s}\n"))
	require.NoError(t, err)
	require.Len(t, sc.Steps, 1)
	step := sc.Steps[0]
	assert.Equal(t, "expect-any", step.Kind())
	assert.Equal(t, []string{"first text", "second text"}, step.ExpectAny)
	assert.Equal(t, 5*time.Second, step.Timeout)
}

func TestExpectAnyNeedsTexts(t *testing.T) {
	for name, step := range map[string]string{
		"an empty list":      "{expect-any: []}",
		"an empty text":      "{expect-any: [\"text\", \"\"]}",
		"a text, not a list": "{expect-any: text}",
		"another action too": "{expect-any: [text], expect: text}",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseScenario("version-any", []byte("description: any text\nargs: []\nsteps:\n  - "+step+"\n"))
			require.Error(t, err)
		})
	}
}

func TestSendRuleAcceptsAnExpectAnyAndTheEndKeys(t *testing.T) {
	key := func(text string) Step { return Step{kind: "send", Send: &text} }
	expect := Step{kind: "expect", Expect: "Name:"}
	expectAny := Step{kind: "expect-any", ExpectAny: []string{"old prompt", "new prompt"}}
	wait := Step{kind: "wait-stdout", WaitStdout: "ready"}

	cases := []struct {
		name  string
		steps []Step
		bad   []int
	}{
		{"keys after an expect", []Step{expect, key("bob\r")}, nil},
		{"keys after an expect-any", []Step{expectAny, key("bob\r")}, nil},
		{"Ctrl-D after keys", []Step{expectAny, key("bob\r"), key("\x04")}, nil},
		{"Ctrl-D written as an escape", []Step{expectAny, key("bob\r"), key(`\x04`)}, nil},
		{"Ctrl-C and Ctrl-D first", []Step{key("\x03"), key("\x04"), key(`\x03`)}, nil},
		{"keys with nothing before", []Step{key("bob\r")}, []int{1}},
		{"keys after keys", []Step{expectAny, key("one\r"), key("two\r"), key("\x04")}, []int{3}},
		{"keys after another kind of wait", []Step{wait, key("bob\r")}, []int{2}},
		{"keys after an empty expect-any", []Step{{kind: "expect-any"}, key("bob\r")}, []int{2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.bad, unwaitedSends(c.steps))
		})
	}
}

func TestPTYExpectAnyContinuesOnEitherText(t *testing.T) {
	for _, texts := range []string{`["never printed", "Name:"]`, `["Name:", "never printed"]`} {
		res, err := runStubScenario(t, "version-pty-any", "args: [prompt]\ntty: true\nsteps:\n"+
			"  - {expect-any: "+texts+"}\n"+
			"  - {send: \"bob\\r\"}\n"+
			"  - {expect: \"hello bob\"}\n")
		require.NoError(t, err, texts)
		assert.Equal(t, "Name: bob\nhello bob\n", res.Stdout, texts)
	}
}

func TestPTYExpectAnyThatNeverMatchesNamesEveryText(t *testing.T) {
	_, err := runStubScenario(t, "version-pty-any-timeout", "args: [prompt]\ntty: true\nsteps:\n"+
		"  - {expect: \"Name:\"}\n"+
		"  - {expect-any: [\"never printed\", \"also never printed\"], timeout: 300ms}\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `stdout to contain one of ["never printed" "also never printed"]`)
	assert.Contains(t, err.Error(), "Name:", "the error shows what the screen held")
}

func TestPTYCtrlDEndsTypedInputWithoutAWait(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-end-of-input", "args: [read-all]\ntty: true\nsteps:\n"+
		"  - {expect-any: [\"Value:\"]}\n"+
		"  - {send: \"abc\\r\"}\n"+
		"  - {send: \"\\x04\"}\n"+
		"  - {expect: \"got\"}\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Contains(t, res.Stdout, "got 4 bytes\n", "the process read abc and the line end that Enter typed")
}
