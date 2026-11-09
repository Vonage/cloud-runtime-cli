package instance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	envelope500 = `{"error":{"code":1001,"message":"boom"}}`
	unknownID   = "00000000-0000-4000-8000-000000000999"
)

// startFake starts the fake platform with its default world and the overrides laid over it.
func startFake(t *testing.T, overrides ...fakeplatform.Override) *fakeplatform.Server {
	t.Helper()
	fx := fakeplatform.Defaults()
	fx.Overrides = append(fx.Overrides, overrides...)
	return fakeplatform.Start(t, fx)
}

// startFakeWith starts the fake platform with the fixtures of yaml laid over the default world.
func startFakeWith(t *testing.T, yaml string) *fakeplatform.Server {
	t.Helper()
	fx, err := fakeplatform.Decode([]byte(yaml))
	require.NoError(t, err)
	return fakeplatform.Start(t, fx)
}

// fixtureID is the id that the default world gives the instance called name.
func fixtureID(t *testing.T, name string) string {
	t.Helper()
	return cmdtest.New(t, startFake(t), cmdtest.Options{}).InstanceID(t, name)
}

// badValue is an id or name that must be refused before any request is made.
type badValue struct{ name, value string }

func badValues() []badValue {
	return []badValue{
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
	go func() { done <- h.Run(t.Context(), instance.NewCmd, args...) }()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the command did not end at the --timeout deadline")
		return nil
	}
}

// instanceBody is the answer of a single-instance route for an instance of the configured region.
func instanceBody(id, serviceName string) string {
	return `{"id":"` + id + `","name":"dev","serviceName":"` + serviceName +
		`","apiApplicationId":"app-1","projectId":"proj-1","regionAlias":"aws.euw1"}`
}
