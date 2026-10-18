package secret_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/secret"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	envelope500 = `{"error":{"code":1001,"message":"boom"}}`

	valueWarning = "! --value is visible to other users and in shell history; pipe the value on stdin or use --filename\n"
	nameError    = `invalid secret name: must follow the regex format "^[a-zA-Z_]+[a-zA-Z0-9_]*$"`
)

// startFake starts the fake platform with its default world and the overrides laid over it.
func startFake(t *testing.T, overrides ...fakeplatform.Override) *fakeplatform.Server {
	t.Helper()
	fx := fakeplatform.Defaults()
	fx.Overrides = append(fx.Overrides, overrides...)
	return fakeplatform.Start(t, fx)
}

// typedPrompt is what a command asks for the value of the named secret.
func typedPrompt(name string) string {
	return `? Enter value for secret "` + name + `" (end with Ctrl-D; for values over 1000 characters use --filename): `
}

func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// runPastTheDeadline runs a command that the fake never answers. The command must end with the error of the
// --timeout deadline; one that ignores the deadline fails the test instead of hanging it.
func runPastTheDeadline(t *testing.T, h *cmdtest.Harness, args ...string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.Run(t.Context(), secret.NewCmd, args...) }()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the command did not end at the --timeout deadline")
		return nil
	}
}
