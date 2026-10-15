package clierr_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	testUsage   = "Usage: vcr x"
	supportLine = "Please refer to the documentation or contact support for further assistance."
)

var separator = strings.Repeat("─", 56)

func render(t *testing.T, err error) (stdout, stderr string, notice bool) {
	t.Helper()
	streams, out, errOut := term.Test("", false)
	notice = clierr.Render(streams, err, testUsage)
	return out.String(), errOut.String(), notice
}

func TestRenderUsage(t *testing.T) {
	stdout, stderr, notice := render(t, clierr.Usagef("bad flag"))

	assert.Equal(t, "bad flag\nUsage: vcr x\n", stderr)
	assert.Empty(t, stdout)
	assert.False(t, notice)
}

func TestRenderUnknownCommand(t *testing.T) {
	stdout, stderr, notice := render(t, errors.New(`unknown command "x" for "vcr"`))

	assert.Equal(t, "unknown command \"x\" for \"vcr\"\nUsage: vcr x\n", stderr)
	assert.Empty(t, stdout)
	assert.False(t, notice)
}

func TestRenderUsageDeepInChain(t *testing.T) {
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", clierr.Usagef("bad flag")))

	stdout, stderr, notice := render(t, err)

	assert.Equal(t, "outer: inner: bad flag\nUsage: vcr x\n", stderr)
	assert.Empty(t, stdout)
	assert.False(t, notice)
}

func TestRenderUsageBranchComesFirst(t *testing.T) {
	err := clierr.Usagef("bad: %w", &clierr.APIError{HTTPStatus: 500})

	stdout, stderr, notice := render(t, err)

	assert.Equal(t, "bad: API Error Encountered: ( HTTP status: 500 )\nUsage: vcr x\n", stderr)
	assert.Empty(t, stdout)
	assert.False(t, notice)
}

func TestRenderAPIError(t *testing.T) {
	err := fmt.Errorf("failed to list applications: %w",
		&clierr.APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1"})

	stdout, stderr, notice := render(t, err)

	assert.Equal(t, separator+"\n"+separator+"\n", stdout)
	assert.Equal(t, "X Error Encountered: failed to list applications\n\n"+
		"ℹ Details:\n"+
		"  - HTTP Status : 500\n"+
		"  - Message     : boom\n"+
		"  - Trace ID    : t-1\n"+
		"\n"+supportLine+"\n", stderr)
	assert.True(t, notice)
}

func TestRenderAPIErrorContainerLogs(t *testing.T) {
	err := fmt.Errorf("failed to list Vonage applications: %w", &clierr.APIError{
		HTTPStatus: 500, Message: "internal error", TraceID: "fake-trace-id", ContainerLogs: "boot failed\nsecond line",
	})

	stdout, stderr, notice := render(t, err)

	assert.Equal(t, separator+"\n"+separator+"\n", stdout)
	assert.Equal(t, "X Error Encountered: failed to list Vonage applications\n\n"+
		"ℹ Details:\n"+
		"  - HTTP Status : 500\n"+
		"  - Message     : internal error\n"+
		"  - Trace ID    : fake-trace-id\n"+
		"\n"+
		"ℹ App logs captured before failure:\n"+
		"boot failed\nsecond line\n"+
		"\n"+supportLine+"\n", stderr)
	assert.True(t, notice)
}

func TestRenderAPIErrorOmitsEmptyFields(t *testing.T) {
	err := fmt.Errorf("failed to do it: %w", &clierr.APIError{ServerCode: 9})

	_, stderr, notice := render(t, err)

	assert.Equal(t, "X Error Encountered: failed to do it\n\nℹ Details:\n\n"+supportLine+"\n", stderr)
	assert.True(t, notice)
}

func TestRenderAPIErrorHeadIsTheTextBeforeTheFirstColon(t *testing.T) {
	api := &clierr.APIError{HTTPStatus: 500}
	tests := []struct {
		name          string
		err           error
		wantFirstLine string
	}{
		{
			name:          "wrapped twice",
			err:           fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", api)),
			wantFirstLine: "X Error Encountered: outer",
		},
		{
			name:          "not wrapped",
			err:           api,
			wantFirstLine: "X Error Encountered: API Error Encountered",
		},
		{
			name:          "no text before the first colon",
			err:           fmt.Errorf(": %w", api),
			wantFirstLine: "X Error Encountered: : API Error Encountered: ( HTTP status: 500 )",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, notice := render(t, tc.err)

			firstLine, _, _ := strings.Cut(stderr, "\n")
			assert.Equal(t, tc.wantFirstLine, firstLine)
			assert.True(t, notice)
		})
	}
}

func TestRenderAPIErrorWritesSeparatorsAroundTheBlock(t *testing.T) {
	streams, _, _ := term.Test("", false)
	var both bytes.Buffer
	streams.Out = &both
	streams.ErrOut = &both

	notice := clierr.Render(streams, fmt.Errorf("failed: %w", &clierr.APIError{HTTPStatus: 500}), testUsage)

	assert.Equal(t, separator+"\n"+
		"X Error Encountered: failed\n\nℹ Details:\n  - HTTP Status : 500\n\n"+supportLine+"\n"+
		separator+"\n", both.String())
	assert.True(t, notice)
}

func TestRenderPlain(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "short message", err: errors.New("x"), want: "X x\n"},
		{name: "colon is not a cut point", err: errors.New("failed: reason"), want: "X failed: reason\n"},
		{name: "percent signs are not verbs", err: errors.New("100% done %s %d"), want: "X 100% done %s %d\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, notice := render(t, tc.err)

			assert.Equal(t, tc.want, stderr)
			assert.Empty(t, stdout)
			assert.True(t, notice)
		})
	}
}

func TestRenderSanitisesMessage(t *testing.T) {
	err := fmt.Errorf("failed to list applications: %w", &clierr.APIError{
		HTTPStatus: 500, Message: "bo\x1b]0;t\aom", TraceID: "t-1", ContainerLogs: "boot\x1b[2J failed",
	})

	stdout, stderr, _ := render(t, err)

	assert.NotContains(t, stdout, "\x1b")
	assert.NotContains(t, stderr, "\x1b")
	assert.Contains(t, stderr, "  - Message     : boom\n")
	assert.Contains(t, stderr, "App logs captured before failure:\nboot failed\n")
}

func TestRenderSanitisesRemoteText(t *testing.T) {
	apiBlock := "ℹ Details:\n  - HTTP Status : 500\n  - Message     : boom\n  - Trace ID    : t-1\n\n" + supportLine + "\n"
	tests := []struct {
		name       string
		err        error
		wantStdout string
		wantStderr string
		wantNotice bool
	}{
		{
			name:       "plain error",
			err:        errors.New("bad \x1b]0;t\a name"),
			wantStderr: "X bad  name\n",
			wantNotice: true,
		},
		{
			name:       "usage error",
			err:        clierr.Usagef("bad \x1b]0;t\a flag"),
			wantStderr: "bad  flag\nUsage: vcr x\n",
		},
		{
			name: "api error head and trace id",
			err: fmt.Errorf("failed\x1b[2J to list: %w",
				&clierr.APIError{HTTPStatus: 500, Message: "boom", TraceID: "t\x1b]0;x\a-1"}),
			wantStdout: separator + "\n" + separator + "\n",
			wantStderr: "X Error Encountered: failed to list\n\n" + apiBlock,
			wantNotice: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, notice := render(t, tc.err)

			assert.NotContains(t, stdout, "\x1b")
			assert.NotContains(t, stderr, "\x1b")
			assert.Equal(t, tc.wantStdout, stdout)
			assert.Equal(t, tc.wantStderr, stderr)
			assert.Equal(t, tc.wantNotice, notice)
		})
	}
}
