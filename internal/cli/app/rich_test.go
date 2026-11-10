package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/app"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// outcome is what a run did to the platform and what it returned.
type outcome struct {
	calls []string
	err   string
}

// runIn runs the command of build on a fresh fake in mode, with the overrides laid over the default world.
func runIn(t *testing.T, mode term.Mode, overrides []fakeplatform.Override, build func(h *cmdtest.Harness) []string) outcome {
	t.Helper()
	h := cmdtest.New(t, startFake(t, overrides...), cmdtest.Options{Mode: mode})
	args := build(h)
	err := h.Run(t.Context(), app.NewCmd, args...)
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
	fixed := func(args ...string) func(*cmdtest.Harness) []string {
		return func(*cmdtest.Harness) []string { return args }
	}
	tests := []struct {
		name      string
		overrides []fakeplatform.Override
		args      func(h *cmdtest.Harness) []string
	}{
		{"create", []fakeplatform.Override{createdOverride()}, fixed("create", "--name", "n", "--yes", "--voice")},
		{"create failing", fail("POST /applications"), fixed("create", "--name", "n", "--yes")},
		{"generate-keys", nil, func(h *cmdtest.Harness) []string { return []string{"generate-keys", "--app-id", h.AppID(t, "my-app")} }},
		{"generate-keys failing", nil, fixed("generate-keys", "--app-id", unknownID)},
		{"list", nil, fixed("list")},
		{"list with a filter", nil, fixed("list", "--filter", "second")},
		{"list failing", fail("GET /applications"), fixed("list")},
		{"remove", nil, func(h *cmdtest.Harness) []string { return []string{"remove", h.AppID(t, "second-app"), "--yes"} }},
		{"remove not found", nil, fixed("remove", unknownID, "--yes")},
		{"remove failing", fail("DELETE /applications/*"), fixed("remove", unknownID, "--yes")},
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
