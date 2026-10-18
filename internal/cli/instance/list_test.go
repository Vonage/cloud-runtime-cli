package instance_test

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

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/instance"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

var listHeaders = []string{"Instance ID", "API Application ID", "Instance Name", "Service Name"}

// table is what term.Table renders for the instance list.
func table(t *testing.T, rows [][]string) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, term.Table(&buf, listHeaders, rows))
	return buf.String()
}

// rowOf is the four cells that the list shows for in.
func rowOf(in platform.Instance) []string {
	return []string{in.ID, in.APIApplicationID, in.Name, in.ServiceName}
}

// listed reads the instances of the fake the way the command will, then clears the request log.
func listed(t *testing.T, h *cmdtest.Harness, filter string) []platform.Instance {
	t.Helper()
	rows, err := h.Env.Client.ListInstances(t.Context(), filter)
	require.NoError(t, err)
	h.Server.Reset()
	return rows
}

func TestListPrintsFourColumnsInServerOrder(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	rows := listed(t, h, "")
	require.Len(t, rows, 3)
	require.Equal(t, []string{"dev", "staging", "prod"}, []string{rows[0].Name, rows[1].Name, rows[2].Name})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, table(t, [][]string{rowOf(rows[0]), rowOf(rows[1]), rowOf(rows[2])}), h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 GET /instances"}, cmdtest.Calls(h.Requests()))
}

func TestListKeepsTheOrderTheServerSent(t *testing.T) {
	body := `{"instances":[` +
		`{"id":"id-zeta","name":"zeta","serviceName":"svc-zeta","apiApplicationId":"app-1","regionAlias":"aws.euw1"},` +
		`{"id":"id-alpha","name":"alpha","serviceName":"svc-alpha","apiApplicationId":"app-2","regionAlias":"aws.use1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, table(t, [][]string{
		{"id-zeta", "app-1", "zeta", "svc-zeta"},
		{"id-alpha", "app-2", "alpha", "svc-alpha"},
	}), h.Stdout.String())
}

func TestListForwardsFilter(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	rows := listed(t, h, "my-project")
	require.Len(t, rows, 2)

	err := h.Run(t.Context(), instance.NewCmd, "list", "--filter", "my-project")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances?filter=my-project"}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, table(t, [][]string{rowOf(rows[0]), rowOf(rows[1])}), h.Stdout.String())
	assert.Contains(t, h.Stdout.String(), "my-project-dev")
	assert.Contains(t, h.Stdout.String(), "my-project-staging")
	assert.NotContains(t, h.Stdout.String(), "other-prod")
}

func TestListShortFilterFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list", "-f", "prod")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances?filter=prod"}, cmdtest.Calls(h.Requests()))
}

func TestListWithoutFilterSendsNoQueryString(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list", "--filter", "")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances"}, cmdtest.Calls(h.Requests()))
}

func TestListEmptyPrintsHeaderOnlyTable(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list", "--filter", "zzz")

	require.NoError(t, err)
	assert.Equal(t, table(t, nil), h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
}

func TestListShowsBlankIdsAsEmptyCells(t *testing.T) {
	body := `{"instances":[{"id":"id-1","name":"customer","serviceName":"svc-1","apiApplicationId":"","projectId":"","regionAlias":"aws.euw1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, table(t, [][]string{{"id-1", "", "customer", "svc-1"}}), h.Stdout.String())
}

func TestListAliasLs(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "ls")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances"}, cmdtest.Calls(h.Requests()))
	assert.Contains(t, h.Stdout.String(), "my-project-dev")
}

func TestListTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list", "extra")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestListSanitisesCells(t *testing.T) {
	body := `{"instances":[{"id":"id-1","name":"\u001b[31mred","serviceName":"svc\u001b]0;pwned\u0007x",` +
		`"apiApplicationId":"app\u001b[2J-1","regionAlias":"aws.euw1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Stdout.String(), "\x1b")
	assert.Equal(t, table(t, [][]string{{"id-1", "app-1", "red", "svcx"}}), h.Stdout.String())
}

func TestListError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list instances: "), "error = %q", err)
	assert.Empty(t, h.Stdout.String(), "no table is printed")
}

func TestListEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "list")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichListTable(t *testing.T) {
	body := `{"instances":[` +
		`{"id":"id-zeta","name":"zeta","serviceName":"svc-zeta","apiApplicationId":"app-1","regionAlias":"aws.euw1"},` +
		`{"id":"id-alpha","name":"alpha","serviceName":"svc-alpha","apiApplicationId":"app-2","regionAlias":"aws.use1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "Instance ID API Application ID Instance Name Service Name\n"+
		"id-zeta app-1 zeta svc-zeta\n"+
		"id-alpha app-2 alpha svc-alpha\n"+
		"\n2 instances\n", cmdtest.Collapse(h.Screen()), "the server's order, the classic header words, a count")
	assert.Empty(t, h.ErrScreen())
	assert.Equal(t, []string{"euw1 GET /instances"}, cmdtest.Calls(h.Requests()))
}

func TestRichListShortensTheNamesAndNeverTheIDs(t *testing.T) {
	longID := strings.Repeat("a", 36)
	longApp := strings.Repeat("b", 36)
	body := `{"instances":[{"id":"` + longID + `","name":"` + strings.Repeat("n", 40) + `","serviceName":"` +
		strings.Repeat("s", 40) + `","apiApplicationId":"` + longApp + `","regionAlias":"aws.euw1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich, Width: 110}) // the ids and the headers of the others need 103

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	row := strings.Split(h.Screen(), "\n")[1]
	assert.True(t, strings.HasPrefix(row, longID+"  "+longApp+"  "), "both id columns are whole: %q", row)
	assert.Contains(t, row, "…", "the name columns give way")
	assert.NotContains(t, row, strings.Repeat("n", 40))
	assert.Equal(t, 110, utf8.RuneCountInString(row), "the table fills the terminal exactly")
}

func TestRichListWiderThanAnyTerminalIsNotElided(t *testing.T) {
	// 36 + 36 columns of ids and the headers of the others come to 103: no cut can make this fit in 100, so none is made.
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: `{"instances":[` +
		`{"id":"` + strings.Repeat("a", 36) + `","name":"dev","serviceName":"my-project-dev","apiApplicationId":"` + strings.Repeat("b", 36) + `","regionAlias":"aws.euw1"},` +
		`{"id":"` + strings.Repeat("c", 36) + `","name":"staging","serviceName":"my-project-staging","apiApplicationId":"` + strings.Repeat("d", 36) + `","regionAlias":"aws.euw1"}]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Screen(), "…")
	assert.Contains(t, h.Screen(), "my-project-dev\n")
	assert.Contains(t, h.Screen(), "my-project-staging\n")
}

func TestRichListEmpty(t *testing.T) {
	srv := startFakeWith(t, "instances: []")
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "No instances found.\n", h.Screen())
	assert.Empty(t, h.ErrScreen())
}

func TestRichListSanitisesCells(t *testing.T) {
	body := `{"instances":[{"id":"id1","name":"a\u001b[2Jb","serviceName":"s\u001b]0;t\u0007v","apiApplicationId":"p","regionAlias":"aws.euw1"}]}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Screen(), "\x1b")
	assert.Contains(t, cmdtest.Collapse(h.Screen()), "id1 p ab sv\n")
}

func TestRichListErrorPrintsNoTable(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list instances: "), "error = %q", err)
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen())
}
