package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const testAppID = "00000000-0000-4000-8000-000000000601"

// harness is one Run over a fake API and a project directory that holds index.js.
type harness struct {
	t      *testing.T
	api    *fakeAPI
	plan   Plan
	poll   *clocktest.Fake // paces the build status poll
	io     *term.IO
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "index.js", "console.log('hello');\n")
	streams, out, errOut := term.Test("", false)
	return &harness{
		t:   t,
		api: newFakeAPI(),
		plan: Plan{
			ProjectDir:   dir,
			Manifest:     &manifest.Manifest{Instance: manifest.Instance{Entrypoint: []string{"node", "index.js"}}},
			ProjectName:  "my-project",
			InstanceName: "dev",
			Runtime:      "nodejs18",
			Region:       "aws.euw1",
			AppID:        testAppID,
		},
		poll:   clocktest.New(time.Unix(0, 0)),
		io:     streams,
		stdout: out,
		stderr: errOut,
	}
}

func (h *harness) run() error { return h.runCtx(context.Background()) }

func (h *harness) runCtx(ctx context.Context) error {
	h.t.Helper()
	return Run(ctx, h.io, h.api, h.poll, h.plan)
}

// tempDirHolder points the temporary directory at an empty one for the test and returns it.
func tempDirHolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func requireEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no temporary file may be left behind")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func hostLine(url string) string { return "| Instance host address: " + term.YellowBold(url) + "\n" }

// wantBlock is the final block for the default fake answer.
func wantBlock() string {
	return "/-------\n" +
		"| Instance has been deployed!\n" +
		"| Instance id: i-1\n" +
		"| Instance service name: my-project-dev\n" +
		hostLine("https://one.example.test") +
		hostLine("https://two.example.test") +
		`\-------` + "\n"
}

func TestRunHappyPath(t *testing.T) {
	h := newHarness(t)

	err := h.run()

	require.NoError(t, err)
	want := `✓ Project "my-project" retrieved: project_id="p-1"` + "\n" +
		"✓ Deployment parameters validated\n" +
		"✓ Source code uploaded.\n" +
		`✓ Package created: package_id="pkg-1"` + "\n" +
		"ℹ Waiting for build to start...\n" +
		"building\n" +
		"Status - completed\n" +
		`✓ Package "pkg-1" built successfully` + "\n" +
		wantBlock()
	assert.Equal(t, want, h.stdout.String())
	assert.Empty(t, h.stderr.String())
	assertBuildCalls(t, h.api, []string{callGetProject, callValidate, callUploadSource, callCreatePackage}, 1)
	assert.Equal(t, "pkg-1", h.api.watchedID)
	assert.Contains(t, h.stdout.String(), "| Instance host address: \x1b[0;1;33mhttps://one.example.test\x1b[0m\n")
}

func TestRunCreatesAMissingProject(t *testing.T) {
	h := newHarness(t)
	h.api.getProjectErr = fmt.Errorf("project: %w", platform.ErrNotFound)
	h.api.newProjectID = "p-new"

	err := h.run()

	require.NoError(t, err)
	assertBuildCalls(t, h.api,
		[]string{callGetProject, callCreateProject, callValidate, callUploadSource, callCreatePackage}, 1)
	assert.Equal(t, "my-project", h.api.createdName)
	assert.Equal(t, "p-new", h.api.validateReq.ProjectID)
	assert.Equal(t, "p-new", h.api.deployReq.ProjectID)
	assert.Contains(t, h.stdout.String(), `✓ Project "my-project" created: project_id="p-new"`+"\n")
	assert.NotContains(t, h.stdout.String(), "retrieved")
}

func TestRunProjectLookupFailure(t *testing.T) {
	h := newHarness(t)
	h.api.getProjectErr = errors.New("boom")

	err := h.run()

	require.EqualError(t, err, `failed to get project details for project "my-project": boom`)
	assert.Equal(t, []string{callGetProject}, h.api.calls)
	assert.Empty(t, h.stdout.String())
}

func TestRunCreateProjectFailure(t *testing.T) {
	h := newHarness(t)
	h.api.getProjectErr = platform.ErrNotFound
	h.api.createErr = errors.New("boom")

	err := h.run()

	require.EqualError(t, err, `failed to create project "my-project": boom`)
	assert.Equal(t, []string{callGetProject, callCreateProject}, h.api.calls)
	assert.Empty(t, h.stdout.String())
}

func TestRunRequiredValuesAreCheckedAtTheirStage(t *testing.T) {
	const parseFailure = "failed to parse capabilities: invalid capability - make sure update is referenced correctly"
	projectLine := `✓ Project "my-project" retrieved: project_id="p-1"` + "\n"
	uploaded := []string{callGetProject, callValidate, callUploadSource}

	tests := []struct {
		name      string
		change    func(*Plan)
		wantErr   string
		wantCalls []string
		wantOut   string
	}{
		{
			name:    "an empty project name stops before any request",
			change:  func(p *Plan) { p.ProjectName = "" },
			wantErr: "failed to get project name: project-name is required",
		},
		{
			name:      "an empty application id stops after the project",
			change:    func(p *Plan) { p.AppID = "" },
			wantErr:   "failed to get instance app id: app-id is required",
			wantCalls: []string{callGetProject},
			wantOut:   projectLine,
		},
		{
			name:      "an empty instance name stops after the project",
			change:    func(p *Plan) { p.InstanceName = "" },
			wantErr:   "failed to get instance name: instance-name is required",
			wantCalls: []string{callGetProject},
			wantOut:   projectLine,
		},
		{
			name:      "an empty application id is reported before an empty instance name",
			change:    func(p *Plan) { p.AppID, p.InstanceName = "", "" },
			wantErr:   "failed to get instance app id: app-id is required",
			wantCalls: []string{callGetProject},
			wantOut:   projectLine,
		},
		{
			name:      "an empty runtime is found after the upload",
			change:    func(p *Plan) { p.Runtime = "" },
			wantErr:   "failed to get runtime: runtime is required",
			wantCalls: uploaded,
			wantOut:   projectLine + "✓ Deployment parameters validated\n✓ Source code uploaded.\n",
		},
		{
			name:      "a capability that does not parse is found after the upload",
			change:    func(p *Plan) { p.Capabilities = []string{"voice-v1-beta"} },
			wantErr:   parseFailure,
			wantCalls: uploaded,
			wantOut:   projectLine + "✓ Deployment parameters validated\n✓ Source code uploaded.\n",
		},
		{
			name: "a capability error comes before the missing runtime",
			change: func(p *Plan) {
				p.Capabilities = []string{"voice-v1-beta"}
				p.Runtime = ""
			},
			wantErr:   parseFailure,
			wantCalls: uploaded,
			wantOut:   projectLine + "✓ Deployment parameters validated\n✓ Source code uploaded.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			tt.change(&h.plan)

			err := h.run()

			require.EqualError(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalls, h.api.calls)
			assert.Equal(t, tt.wantOut, h.stdout.String())
			assert.Empty(t, h.stderr.String())
		})
	}
}

func TestRunValidationFailure(t *testing.T) {
	h := newHarness(t)
	h.api.validation = platform.ValidateResult{Errors: []platform.ValidationError{
		{Field: "instanceName", Message: "too long"},
		{Field: "region", Message: "unknown"},
	}}

	err := h.run()

	require.EqualError(t, err, "Deployment validation failed:\n  - instanceName: too long\n  - region: unknown")
	assert.Equal(t, []string{callGetProject, callValidate}, h.api.calls)
	assert.NotContains(t, h.stdout.String(), "validated")
}

func TestRunValidationErrorsAreSanitised(t *testing.T) {
	h := newHarness(t)
	h.api.validation = platform.ValidateResult{Errors: []platform.ValidationError{
		{Field: "instance\x1b[2JName", Message: "\x1b]0;title\atoo long\x1b[31m"},
	}}

	err := h.run()

	require.EqualError(t, err, "Deployment validation failed:\n  - instanceName: too long")
}

func TestRunValidateFailure(t *testing.T) {
	h := newHarness(t)
	h.api.validateErr = errors.New("boom")

	err := h.run()

	require.EqualError(t, err, "failed to validate deployment: boom")
	assert.Equal(t, []string{callGetProject, callValidate}, h.api.calls)
}

func TestRunStopsWhenTheContextEnds(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.api.onValidate = cancel
	tmp := tempDirHolder(t)

	err := h.runCtx(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to compress and upload file: failed to compress directory "),
		"got %q", err.Error())
	assert.False(t, h.api.called(callUploadSource))
	assert.False(t, h.api.called(callCreatePackage))
	requireEmptyDir(t, tmp)
}

// assertBuildCalls checks a deploy that went through the build: the calls before it in order, then the watch once
// and statuses status reads in any order, then the deploy.
func assertBuildCalls(t *testing.T, api *fakeAPI, before []string, statuses int) {
	t.Helper()
	calls := api.calls
	require.Greater(t, len(calls), len(before), "calls: %q", calls)
	assert.Equal(t, before, calls[:len(before)])
	build := slices.Clone(calls[len(before) : len(calls)-1])
	slices.Sort(build)
	want := []string{callWatchBuild}
	for range statuses {
		want = append(want, callBuildStatus)
	}
	slices.Sort(want)
	assert.Equal(t, want, build, "the build stage reads the status and watches once, in either order")
	assert.Equal(t, callDeployInstance, calls[len(calls)-1])
}
