package deploy

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

var richStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// lines splits screen text into its lines and drops the empty one after the last newline.
func lines(s string) []string {
	out := strings.Split(s, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func TestRichDeployAgainstTheFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	streams, stdout, stderr := term.TestRich(term.Rich, 100, clocktest.New(richStart))
	env.IO = streams
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.NoError(t, err)
	want := []string{
		"✓ Project resolved  my-project",
		"✓ Parameters validated",
		"✓ Files packaged  2 files",
		"✓ Source uploaded",
		"✓ Package created  <id>",
		"  │ Starting build",
		"  │ Step 1/2 : install dependencies",
		"  │ ℹ Status - completed",
		"✓ Package built",
		"✓ Instance deployed",
		"",
		"✓ Instance has been deployed!",
		"  Instance id: <id>",
		"  Instance service name: my-project-dev",
		"  Instance host address: https://my-project-dev.euw1.runtime.example.test",
	}
	assert.Equal(t, want, lines(fixtureID.ReplaceAllString(stdout.Screen(), "<id>")))
	assert.Contains(t, stdout.Raw(), "\x1b[35mℹ\x1b[0m Status - completed", "the server's colour is untouched")
	assert.Empty(t, stderr.Raw())
	assert.Equal(t, []string{
		"GET /regions/aws.euw1",
		"GET /projects/my-project",
		"POST /deployments/validate",
		"POST /packages/source",
		"POST /packages",
		"GET /packages/<id>/build/status",
		"GET /packages/<id>/build/watch",
		"POST /deployments",
	}, deployRequestsOf(srv), "the classic requests, and the status read")
}
