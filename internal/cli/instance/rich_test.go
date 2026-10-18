package instance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/instance"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// outcome is what a run did to the platform and what it returned.
type outcome struct {
	calls []string
	err   string
}

func runIn(t *testing.T, mode term.Mode, overrides []fakeplatform.Override, build func(h *cmdtest.Harness) []string) outcome {
	t.Helper()
	h := cmdtest.New(t, startFake(t, overrides...), cmdtest.Options{Mode: mode})
	args := build(h)
	err := h.Run(t.Context(), instance.NewCmd, args...)
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
	byID := func(name string) func(*cmdtest.Harness) []string {
		return func(h *cmdtest.Harness) []string { return []string{"remove", "--id", h.InstanceID(t, name), "--yes"} }
	}
	tests := []struct {
		name      string
		overrides []fakeplatform.Override
		args      func(h *cmdtest.Harness) []string
	}{
		{"list", nil, fixed("list")},
		{"list with a filter", nil, fixed("list", "--filter", "dev")},
		{"list failing", fail("GET /instances"), fixed("list")},
		{"remove by id", nil, byID("dev")},
		{"remove in another region", nil, byID("prod")},
		{"remove by names", nil, fixed("remove", "-p", "my-project", "-n", "dev", "-y")},
		{"remove not found", nil, fixed("remove", "--yes", "--id", unknownID)},
		{"remove lookup failing", fail("GET /instances/*"), fixed("remove", "--yes", "--id", unknownID)},
		{"remove failing", fail("DELETE /instances/*"), byID("dev")},
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
