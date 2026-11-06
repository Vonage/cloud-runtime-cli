package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// jsonBody decodes the body of a request the fake received.
func jsonBody(t *testing.T, r fakeplatform.Request) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(r.Body, &out), "body: %s", r.Body)
	return out
}

func TestDeployAgainstTheFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, stderr := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.NoError(t, err)
	want := `✓ Project "my-project" retrieved: project_id="<id>"` + "\n" +
		"✓ Deployment parameters validated\n" +
		"✓ Source code uploaded.\n" +
		`✓ Package created: package_id="<id>"` + "\n" +
		"ℹ Waiting for build to start...\n" +
		"Starting build\n" +
		"Step 1/2 : install dependencies\n" +
		term.SanitizeStream("\x1b[35mℹ\x1b[0m Status - completed") + "\n" +
		`✓ Package "<id>" built successfully` + "\n" +
		"/-------\n" +
		"| Instance has been deployed!\n" +
		"| Instance id: <id>\n" +
		"| Instance service name: my-project-dev\n" +
		"| Instance host address: " + term.YellowBold("https://my-project-dev.euw1.runtime.example.test") + "\n" +
		`\-------` + "\n"
	assert.Equal(t, want, fixtureID.ReplaceAllString(stdout.String(), "<id>"))
	assert.Empty(t, stderr.String())
	assert.Equal(t, []string{
		"GET /regions/aws.euw1",
		"GET /projects/my-project",
		"POST /deployments/validate",
		"POST /packages/source",
		"POST /packages",
		"GET /packages/<id>/build/status",
		"GET /packages/<id>/build/watch",
		"POST /deployments",
	}, deployRequestsOf(srv))
	for _, r := range srv.Requests() {
		assert.Equal(t, testKey, r.User, "%s %s", r.Method, r.Path)
	}
}

func TestDeployRequestBodies(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.NoError(t, err)
	validate := jsonBody(t, requestNamed(t, srv, "POST /deployments/validate"))
	assert.NotEmpty(t, validate["projectId"])
	delete(validate, "projectId")
	assert.Equal(t, map[string]any{
		"apiApplicationId": testAppID,
		"environment":      nil,
		"instanceName":     "dev",
		"maxScale":         float64(0),
		"minScale":         float64(0),
		"region":           "aws.euw1",
	}, validate)

	create := requestNamed(t, srv, "POST /packages")
	require.JSONEq(t,
		`{"sourceCodeKey":"source-key-0001","entrypoint":["node","index.js"],"capabilities":{},"buildScriptPath":"","runtime":"nodejs18"}`,
		string(create.Body))

	deployed := jsonBody(t, requestNamed(t, srv, "POST /deployments"))
	assert.NotEmpty(t, deployed["projectId"])
	assert.NotEmpty(t, deployed["packageId"])
	assert.Equal(t, testAppID, deployed["apiApplicationId"])
	assert.Equal(t, "dev", deployed["instanceName"])
	assert.Equal(t, "aws.euw1", deployed["region"])
	assert.NotContains(t, deployed, "security")
	assert.NotContains(t, deployed, "healthCheckEndpoint")
	assert.Contains(t, deployed, "domains")
	assert.Nil(t, deployed["domains"])
	assert.Contains(t, deployed, "environment")
	assert.Nil(t, deployed["environment"])
}

func TestDeployRequestBodiesCarryTheManifestSections(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	extra := "  environment:\n    - name: A\n      value: \"1\"\n" +
		"  domains: [api.example.test]\n  build-script: ./build.sh\n  health-check-path: /health\n" +
		"  scaling:\n    min-scale: 1\n    max-scale: 3\n  security:\n    access: private\n"
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", extra))

	err := execute(t, env, dir)

	require.NoError(t, err)
	deployed := jsonBody(t, requestNamed(t, srv, "POST /deployments"))
	assert.Equal(t, []any{map[string]any{"name": "A", "value": "1", "secret": ""}}, deployed["environment"])
	assert.Equal(t, []any{"api.example.test"}, deployed["domains"])
	assert.Equal(t, "/health", deployed["healthCheckEndpoint"])
	assert.Equal(t, float64(1), deployed["minScale"])
	assert.Equal(t, float64(3), deployed["maxScale"])
	assert.Equal(t, map[string]any{"access": "private"}, deployed["security"])
	create := jsonBody(t, requestNamed(t, srv, "POST /packages"))
	assert.Equal(t, "./build.sh", create["buildScriptPath"])
}

func TestFlagsOverrideTheManifest(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", "  capabilities: [rtc]\n"))
	const otherApp = "00000000-0000-4000-8000-000000000602"

	err := execute(t, env, dir, "-p", "other", "-n", "qa", "-r", "python3", "-i", otherApp, "-c", "voice,rtc")

	require.NoError(t, err)
	assert.Contains(t, requestsOf(srv), "GET /projects/other")
	assert.NotContains(t, requestsOf(srv), "GET /projects/my-project")
	validate := jsonBody(t, requestNamed(t, srv, "POST /deployments/validate"))
	assert.Equal(t, "qa", validate["instanceName"])
	assert.Equal(t, otherApp, validate["apiApplicationId"])
	create := requestNamed(t, srv, "POST /packages")
	got := jsonBody(t, create)
	assert.Equal(t, "python3", got["runtime"])
	assert.Equal(t, map[string]any{"voice": "v0", "rtc": "v0"}, got["capabilities"])
}

func TestMissingProjectIsCreated(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("brand-new", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.NoError(t, err)
	requests := requestsOf(srv)
	assert.Equal(t, []string{"GET /regions/aws.euw1", "GET /projects/brand-new", "POST /projects"}, requests[:3])
	assert.JSONEq(t, `{"name":"brand-new","origin":"owner"}`, string(requestNamed(t, srv, "POST /projects").Body))
	assert.Contains(t, stdout.String(), `✓ Project "brand-new" created: project_id="`)
	assert.NotContains(t, stdout.String(), "retrieved")
}

func TestBadCapabilityFailsAfterTheUpload(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", "  capabilities: [voice-v1-beta]\n"))

	err := execute(t, env, dir)

	require.EqualError(t, err, "failed to parse capabilities: invalid capability - make sure update is referenced correctly")
	requests := requestsOf(srv)
	assert.Equal(t, "POST /packages/source", requests[len(requests)-1])
	assert.NotContains(t, requests, "POST /packages")
	assert.Contains(t, stdout.String(), "✓ Source code uploaded.\n")
}

func TestFilenameFlagIsUsedAsTyped(t *testing.T) {
	t.Run("an absolute path", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		env, _, _ := newEnv(t, srv, defaultSettings(), good)
		dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))
		custom := filepath.Join(t.TempDir(), "elsewhere.yml")
		require.NoError(t, os.WriteFile(custom, []byte(manifestYAML("other", "aws.euw1", "")), 0o600))

		err := execute(t, env, dir, "--filename", custom)

		require.NoError(t, err)
		assert.Contains(t, requestsOf(srv), "GET /projects/other")
	})

	t.Run("a relative path is relative to the working directory, not the project", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		env, _, _ := newEnv(t, srv, defaultSettings(), good)
		dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))
		work := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(work, "custom.yml"), []byte(manifestYAML("other", "aws.euw1", "")), 0o600))
		t.Chdir(work)

		err := execute(t, env, dir, "-f", "custom.yml")

		require.NoError(t, err)
		assert.Contains(t, requestsOf(srv), "GET /projects/other")
	})

	t.Run("a missing file makes no request", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		env, _, _ := newEnv(t, srv, defaultSettings(), good)
		dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

		err := execute(t, env, dir, "-f", "no-such-file.yml")

		require.EqualError(t, err, "failed to find flag manifest file: manifest file not found")
		assert.Empty(t, requestsOf(srv))
	})
}

func TestTgzFlagUploadsTheArchiveAsTyped(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))
	archive := filepath.Join(t.TempDir(), "app.tar.gz")
	require.NoError(t, os.WriteFile(archive, []byte{0x1f, 0x8b, 0x08, 0x00}, 0o600))

	err := execute(t, env, dir, "--tgz", archive)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "✓ Source code uploaded.\n")
	assert.Contains(t, requestsOf(srv), "POST /packages/source")
}
