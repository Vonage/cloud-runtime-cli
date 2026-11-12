package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// calls is everything a run asked of the platform: the methods in order and what each was given. The path of an
// uploaded temporary archive is left out because it differs from run to run.
type calls struct {
	Methods      []string
	ProjectAsked string
	CreatedName  string
	Validate     platform.ValidateRequest
	Package      platform.PackageRequest
	Deploy       platform.DeployRequest
	WatchedID    string
	Entries      []string
}

func callsOf(f *fakeAPI) calls {
	return calls{
		Methods: buildStageSorted(f.calls), ProjectAsked: f.projectAsked, CreatedName: f.createdName, Validate: f.validateReq,
		Package: f.packageReq, Deploy: f.deployReq, WatchedID: f.watchedID, Entries: f.entries,
	}
}

// outcome is what one presentation of a deploy did.
type outcome struct {
	err            error
	calls          calls
	stdout, stderr string
}

// runIn runs the stages in the presentation, over a fresh fake API that configure sets up, and checks that no
// temporary file is left behind.
func runIn(t *testing.T, mode term.Mode, plan Plan, configure func(*fakeAPI)) outcome {
	t.Helper()
	tmp := tempDirHolder(t)
	api := newFakeAPI()
	if configure != nil {
		configure(api)
	}
	var (
		err            error
		stdout, stderr string
	)
	if mode == term.Classic {
		streams, out, errOut := term.Test("", false)
		err = Run(t.Context(), streams, api, clocktest.New(richStart), plan)
		stdout, stderr = out.String(), errOut.String()
	} else {
		streams, out, errOut := term.TestRich(mode, 80, clocktest.New(richStart))
		err = Run(t.Context(), streams, api, clocktest.New(richStart), plan)
		stdout, stderr = out.Screen(), errOut.Screen()
	}
	requireEmptyDir(t, tmp)
	return outcome{err: err, calls: callsOf(api), stdout: stdout, stderr: stderr}
}

// failure is a deploy that must fail the same way in both presentations.
type failure struct {
	name       string
	plan       func(t *testing.T, p *Plan) // changes the plan once, before both runs
	api        func(a *fakeAPI)            // sets up the fake of each run
	unresolved string                      // text of the resolved line the failed step must not leave
}

func notGzipFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain.tgz")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))
	return path
}

func failures() []failure {
	boom := errors.New("boom")
	return []failure{
		{name: "project lookup", api: func(a *fakeAPI) { a.getProjectErr = boom }, unresolved: "Project resolved"},
		{
			name:       "project creation",
			api:        func(a *fakeAPI) { a.getProjectErr, a.createErr = platform.ErrNotFound, boom },
			unresolved: "Project created",
		},
		{name: "empty project name", plan: func(_ *testing.T, p *Plan) { p.ProjectName = "" }, unresolved: "Project resolved"},
		{name: "empty application id", plan: func(_ *testing.T, p *Plan) { p.AppID = "" }, unresolved: "Parameters validated"},
		{name: "empty instance name", plan: func(_ *testing.T, p *Plan) { p.InstanceName = "" }, unresolved: "Parameters validated"},
		{name: "validation fails", api: func(a *fakeAPI) { a.validateErr = boom }, unresolved: "Parameters validated"},
		{
			name: "validation refuses",
			api: func(a *fakeAPI) {
				a.validation = platform.ValidateResult{Errors: []platform.ValidationError{
					{Field: "instanceName", Message: "too long"}, {Field: "region", Message: "unknown"},
				}}
			},
			unresolved: "Parameters validated",
		},
		{name: "upload fails", api: func(a *fakeAPI) { a.uploadErr = boom }, unresolved: "Source uploaded"},
		{
			name: "nothing to upload",
			plan: func(t *testing.T, p *Plan) {
				t.Helper()
				p.ProjectDir = t.TempDir()
				writeFile(t, p.ProjectDir, ".vcrignore", "ignored.txt\n")
				writeFile(t, p.ProjectDir, "ignored.txt", "left out\n")
			},
			unresolved: "Files packaged",
		},
		{
			name:       "capability does not parse",
			plan:       func(_ *testing.T, p *Plan) { p.Capabilities = []string{"voice-v1-beta"} },
			unresolved: "Package created",
		},
		{name: "empty runtime", plan: func(_ *testing.T, p *Plan) { p.Runtime = "" }, unresolved: "Package created"},
		{name: "package creation", api: func(a *fakeAPI) { a.packageErr = boom }, unresolved: "Package created"},
		{
			name:       "build fails",
			api:        func(a *fakeAPI) { a.statuses = []statusAnswer{{status: "failed"}} },
			unresolved: "Package built",
		},
		{name: "instance deploy", api: func(a *fakeAPI) { a.deployErr = boom }, unresolved: "Instance deployed"},
		{
			name: "tgz is not gzip",
			plan: func(t *testing.T, p *Plan) {
				t.Helper()
				p.TgzPath = notGzipFile(t)
			},
			unresolved: "Source uploaded",
		},
		{
			name: "tgz upload fails",
			plan: func(t *testing.T, p *Plan) {
				t.Helper()
				p.TgzPath = writeTgz(t, t.TempDir(), "packed.js")
			},
			api:        func(a *fakeAPI) { a.uploadErr = boom },
			unresolved: "Source uploaded",
		},
	}
}

func TestRichDeployErrorsMatchClassic(t *testing.T) {
	for _, tt := range failures() {
		t.Run(tt.name, func(t *testing.T) {
			plan := newHarness(t).plan
			if tt.plan != nil {
				tt.plan(t, &plan)
			}

			classic := runIn(t, term.Classic, plan, tt.api)
			rich := runIn(t, term.Rich, plan, tt.api)

			require.Error(t, classic.err)
			require.Error(t, rich.err)
			assert.Equal(t, classic.err.Error(), rich.err.Error())
			assert.Equal(t, classic.calls, rich.calls, "the platform is asked the same, in the same order")
			assert.NotContains(t, rich.stdout, "✗", "clierr prints the failure once")
			assert.NotContains(t, rich.stderr, "✗", "a failed step is erased, not printed as a failure")
			assert.NotContains(t, rich.stdout, tt.unresolved, "a step that failed leaves no resolved line")
		})
	}
}

// fullInstance sets every section of the manifest that reaches a request.
func fullInstance(_ *testing.T, p *Plan) {
	p.Manifest.Instance = manifest.Instance{
		Entrypoint:      []string{"node", "index.js"},
		Environment:     []manifest.Env{{Name: "A", Value: "1"}, {Name: "B", Secret: "shh"}},
		Domains:         []string{"api.example.test"},
		BuildScript:     "./build.sh",
		Scaling:         manifest.Scaling{MinScale: 1, MaxScale: 3},
		Security:        &manifest.Security{Access: "private", AuthMethod: "jwt"},
		HealthCheckPath: "/health",
	}
	p.Capabilities = []string{"messages-v1", "voice", "rtc"}
}

func TestRichDeployRequestsMatchClassic(t *testing.T) {
	tests := []struct {
		name string
		plan func(t *testing.T, p *Plan)
	}{
		{"a bare manifest leaves its lists nil", nil},
		{"every section of the manifest is carried", fullInstance},
		{"a project that is created", func(_ *testing.T, p *Plan) { p.ProjectName = "brand-new" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := newHarness(t).plan
			if tt.plan != nil {
				tt.plan(t, &plan)
			}
			configure := func(a *fakeAPI) {
				if plan.ProjectName == "brand-new" {
					a.getProjectErr, a.newProjectID = platform.ErrNotFound, "p-new"
				}
			}

			classic := runIn(t, term.Classic, plan, configure)
			rich := runIn(t, term.Rich, plan, configure)

			require.NoError(t, classic.err)
			require.NoError(t, rich.err)
			assert.Equal(t, classic.calls, rich.calls)
			if tt.plan == nil {
				assert.Nil(t, rich.calls.Validate.Environment)
				assert.Nil(t, rich.calls.Deploy.Environment)
				assert.Nil(t, rich.calls.Deploy.Domains)
			}
		})
	}
}

func TestStaticDeployMatchesRichScreen(t *testing.T) {
	rich := newRichRun(t, term.Rich)
	static := newRichRun(t, term.Static)
	rich.api.frames = buildOf(scriptLines(1, 3))
	static.api.frames = buildOf(scriptLines(1, 3))

	require.NoError(t, rich.run())
	require.NoError(t, static.run())

	// The static screen keeps the script lines the rich one erased.
	var script, rest []string
	for _, line := range lines(static.stdout.Screen()) {
		if strings.HasPrefix(line, "  │") {
			script = append(script, line)
		} else {
			rest = append(rest, line)
		}
	}
	assert.Equal(t, guttered(scriptLines(1, 3)), script)
	assert.Equal(t, lines(rich.stdout.Screen()), rest)
	assert.Equal(t, rich.stderr.Screen(), static.stderr.Screen())
	for _, buffer := range []*term.TermBuffer{static.stdout, static.stderr} {
		for _, sequence := range []string{"\r", "\x1b[2K", "\x1b[?25l"} {
			assert.NotContains(t, buffer.Raw(), sequence)
		}
	}
}

// buildStageSorted sorts the calls the build stage makes from two goroutines, whose order is not fixed.
func buildStageSorted(in []string) []string {
	out := slices.Clone(in)
	start := slices.Index(out, callCreatePackage) + 1
	if start == 0 {
		return out
	}
	end := len(out)
	if i := slices.Index(out, callDeployInstance); i >= 0 {
		end = i
	}
	slices.Sort(out[start:end])
	return out
}
