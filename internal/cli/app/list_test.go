package app_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/app"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

var listHeaders = []string{"ID", "Name"}

// table is what term.Table renders for the application list.
func table(t *testing.T, rows [][]string) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, term.Table(&buf, listHeaders, rows))
	return buf.String()
}

func TestListPrintsTableInServerOrder(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	myApp, second := h.AppID(t, "my-app"), h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, table(t, [][]string{{myApp, "my-app"}, {second, "second-app"}}), h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 GET /applications?pattern="}, cmdtest.Calls(h.Requests()))
}

func TestListHeaderIsUpperCasedByTheTable(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	lines := strings.Split(h.Stdout.String(), "\n")
	require.Greater(t, len(lines), 2)
	assert.Contains(t, lines[1], "ID")
	assert.Contains(t, lines[1], "NAME")
}

func TestListKeepsServerOrder(t *testing.T) {
	body := `{"applications":[{"id":"id-zeta","name":"zeta"},{"id":"id-alpha","name":"alpha"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, table(t, [][]string{{"id-zeta", "zeta"}, {"id-alpha", "alpha"}}), h.Stdout.String())
}

func TestListSendsFilterAsPattern(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	second := h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "list", "--filter", "second")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /applications?pattern=second"}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, table(t, [][]string{{second, "second-app"}}), h.Stdout.String())
}

func TestListShortFilterFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list", "-f", "my")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /applications?pattern=my"}, cmdtest.Calls(h.Requests()))
}

func TestListEmptyPrintsHeaderOnlyTable(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list", "--filter", "zzz")

	require.NoError(t, err)
	assert.Equal(t, table(t, nil), h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
}

func TestListAliasLs(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "ls")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /applications?pattern="}, cmdtest.Calls(h.Requests()))
	assert.Contains(t, h.Stdout.String(), "my-app")
}

func TestListTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list", "extra")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestListSanitisesCells(t *testing.T) {
	body := `{"applications":[{"id":"i\u001b[31md","name":"\u001b[31mred"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Stdout.String(), "\x1b")
	assert.Equal(t, table(t, [][]string{{"id", "red"}}), h.Stdout.String())
}

func TestListError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list Vonage applications: "), "error = %q", err)
	assert.Empty(t, h.Stdout.String(), "no table is printed")
}

func TestListEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "list")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichListTable(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})
	myApp, second := h.AppID(t, "my-app"), h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "ID Name\n"+myApp+" my-app\n"+second+" second-app\n\n2 applications\n", cmdtest.Collapse(h.Screen()))
	assert.Empty(t, h.ErrScreen())
	assert.Equal(t, []string{"euw1 GET /applications?pattern="}, cmdtest.Calls(h.Requests()))
}

func TestRichListKeepsTheIDWholeOnANarrowTerminal(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Body: `{"applications":[{"id":"` + createdID + `","name":"` + strings.Repeat("n", 90) + `"}]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich, Width: 60})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	row := strings.Split(h.Screen(), "\n")[1]
	assert.True(t, strings.HasPrefix(row, createdID+"  "), "the id is whole: %q", row)
	assert.True(t, strings.HasSuffix(row, "…"), "the name is the column that gives way: %q", row)
	assert.Equal(t, 60, utf8.RuneCountInString(row), "the table fills the terminal exactly")
}

func TestRichListEmpty(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), app.NewCmd, "list", "--filter", "zzz")

	require.NoError(t, err)
	assert.Equal(t, "No applications found.\n", h.Screen())
	assert.Empty(t, h.ErrScreen())
}

func TestRichListSanitisesCells(t *testing.T) {
	body := `{"applications":[{"id":"i\u001b]0;x\u0007d","name":"a\u001b[2Jb"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Screen(), "\x1b")
	assert.Equal(t, "ID Name\nid ab\n\n1 application\n", cmdtest.Collapse(h.Screen()))
}

func TestRichListErrorPrintsNoTable(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /applications", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), app.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list Vonage applications: "), "error = %q", err)
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen())
}
