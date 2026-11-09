package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/app"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	envelope500 = `{"error":{"code":1001,"message":"boom"}}`
	unknownID   = "00000000-0000-4000-8000-000000000000"
)

// startFake starts the fake platform with its default world and the overrides laid over it.
func startFake(t *testing.T, overrides ...fakeplatform.Override) *fakeplatform.Server {
	t.Helper()
	fx := fakeplatform.Defaults()
	fx.Overrides = append(fx.Overrides, overrides...)
	return fakeplatform.Start(t, fx)
}

// badID is a value that must be refused before any request is made.
type badID struct{ name, id string }

func badIDs() []badID {
	return []badID{
		{"slash", "a/b"},
		{"backslash", `a\b`},
		{"percent", "a%2Fb"},
		{"double dot", ".."},
		{"dot", "."},
		{"over 256 bytes", strings.Repeat("a", 257)},
		{"control character", "a\x07b"},
	}
}

// runPastTheDeadline runs a command that the fake never answers. The command must end with the error of the
// --timeout deadline; one that ignores the deadline fails the test instead of hanging it.
func runPastTheDeadline(t *testing.T, h *cmdtest.Harness, args ...string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.Run(t.Context(), app.NewCmd, args...) }()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the command did not end at the --timeout deadline")
		return nil
	}
}
