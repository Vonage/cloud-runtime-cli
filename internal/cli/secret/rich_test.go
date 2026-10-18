package secret_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/secret"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestRichCreateUpdateRemoveLines(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"create", []string{"create", "--name", "MY_KEY", "--value", "abc"}, "✓ Secret \"MY_KEY\" created\n"},
		{"update", []string{"update", "--name", "DB_URL", "--value", "abc"}, "✓ Secret \"DB_URL\" updated\n"},
		{"remove", []string{"remove", "--name", "DB_URL"}, "✓ Secret \"DB_URL\" successfully removed\n"},
	}
	modes := []struct {
		name string
		mode term.Mode
	}{{"rich", term.Rich}, {"static", term.Static}}
	for _, m := range modes {
		for _, tt := range tests {
			t.Run(m.name+" "+tt.name, func(t *testing.T) {
				h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: m.mode})

				err := h.Run(t.Context(), secret.NewCmd, tt.args...)

				require.NoError(t, err)
				assert.Equal(t, tt.want, cmdtest.Collapse(h.Screen()))
				assert.NotContains(t, h.Screen(), "abc", "the value is never printed")
			})
		}
	}
}

// outcome is what a run did to the platform and what it returned.
type outcome struct {
	calls []string
	err   string
}

func runIn(t *testing.T, mode term.Mode, overrides []fakeplatform.Override, args []string) outcome {
	t.Helper()
	h := cmdtest.New(t, startFake(t, overrides...), cmdtest.Options{Mode: mode})
	err := h.Run(t.Context(), secret.NewCmd, args...)
	got := outcome{calls: cmdtest.Calls(h.Requests())}
	if err != nil {
		got.err = err.Error()
	}
	return got
}

func TestRichAndClassicAgree(t *testing.T) {
	fail := func(match string) []fakeplatform.Override {
		return []fakeplatform.Override{{Match: match, Status: http.StatusInternalServerError, Body: envelope500}}
	}
	tests := []struct {
		name      string
		overrides []fakeplatform.Override
		args      []string
	}{
		{"create", nil, []string{"create", "--name", "MY_KEY", "--value", "abc"}},
		{"create conflict", nil, []string{"create", "--name", "DB_URL", "--value", "abc"}},
		{"create failing", fail("POST /secrets"), []string{"create", "--name", "MY_KEY", "--value", "abc"}},
		{"update", nil, []string{"update", "--name", "DB_URL", "--value", "abc"}},
		{"update not found", nil, []string{"update", "--name", "NOPE", "--value", "abc"}},
		{"update failing", fail("PATCH /secrets"), []string{"update", "--name", "DB_URL", "--value", "abc"}},
		{"remove", nil, []string{"remove", "--name", "DB_URL"}},
		{"remove failing", fail("DELETE /secrets/*"), []string{"remove", "--name", "DB_URL"}},
		{"list", nil, []string{"list"}},
		{"list failing", fail("GET /secrets"), []string{"list"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classic := runIn(t, term.Classic, tt.overrides, tt.args)
			rich := runIn(t, term.Rich, tt.overrides, tt.args)

			require.NotEmpty(t, classic.calls, "the command reached the platform")
			assert.Equal(t, classic.calls, rich.calls, "the same requests")
			assert.Equal(t, classic.err, rich.err, "the same error")
		})
	}
}
