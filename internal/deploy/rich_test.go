package deploy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

var (
	richStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	spaceRuns = regexp.MustCompile(` {2,}`)
)

// richRun is one Run in the interactive presentation over a fake API, with the buffers that hold what it printed.
type richRun struct {
	t      *testing.T
	api    *fakeAPI
	plan   Plan
	clk    *clocktest.Fake
	io     *term.IO
	stdout *term.TermBuffer
	stderr *term.TermBuffer
}

func newRichRun(t *testing.T, mode term.Mode) *richRun {
	t.Helper()
	base := newHarness(t)
	clk := clocktest.New(richStart)
	streams, stdout, stderr := term.TestRich(mode, 80, clk)
	return &richRun{t: t, api: base.api, plan: base.plan, clk: clk, io: streams, stdout: stdout, stderr: stderr}
}

func (r *richRun) run() error {
	r.t.Helper()
	return Run(r.t.Context(), r.io, r.api, r.plan)
}

// moveClock makes the fake clock pass d inside the named calls of the fake API.
func (r *richRun) moveClock(by map[string]time.Duration) {
	r.api.onCall = func(name string) {
		if d, ok := by[name]; ok {
			_ = r.clk.Sleep(context.Background(), d)
		}
	}
}

// lines splits screen text into its lines and drops the empty one after the last newline.
func lines(s string) []string {
	out := strings.Split(s, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// squash folds each run of spaces inside a line into one, keeping the indentation, so a test does not depend on how
// far apart a noun and its dim detail are.
func squash(line string) string {
	text := strings.TrimLeft(line, " ")
	return line[:len(line)-len(text)] + spaceRuns.ReplaceAllString(text, " ")
}

func squashed(in []string) []string {
	out := make([]string, len(in))
	for i, line := range in {
		out[i] = squash(line)
	}
	return out
}

// lineWith returns the first line that contains text.
func lineWith(t *testing.T, in []string, text string) string {
	t.Helper()
	for _, line := range in {
		if strings.Contains(line, text) {
			return line
		}
	}
	require.FailNow(t, "no such line", "%q in %q", text, in)
	return ""
}

// blockFrom returns the lines from the one that contains text on.
func blockFrom(t *testing.T, in []string, text string) []string {
	t.Helper()
	for i, line := range in {
		if strings.Contains(line, text) {
			return in[i:]
		}
	}
	require.FailNow(t, "no such line", "%q in %q", text, in)
	return nil
}

func TestRichDeployResolvesEachStage(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.deployed.HostURLs = []string{"https://one.example.test"}

	require.NoError(t, r.run())

	assert.Equal(t, []string{
		"✓ Project resolved my-project",
		"✓ Parameters validated",
		"✓ Files packaged 1 file",
		"✓ Source uploaded",
		"✓ Package created pkg-1",
		"  │ building",
		"  │ Status - completed",
		"✓ Package built",
		"✓ Instance deployed",
		"",
		"✓ Instance has been deployed!",
		"  Instance id: i-1",
		"  Instance service name: my-project-dev",
		"  Instance host address: https://one.example.test",
	}, squashed(lines(r.stdout.Screen())))
	assert.Empty(t, r.stderr.Raw())
	for _, classic := range []string{"Waiting for build", "built successfully", "/-------", "retrieved", "Compressing"} {
		assert.NotContains(t, r.stdout.Raw(), classic)
	}
}

func TestRichDeployFastStepsDrawNoLiveFrame(t *testing.T) {
	r := newRichRun(t, term.Rich)

	require.NoError(t, r.run())

	assert.Empty(t, r.stderr.Raw())
	for _, sequence := range []string{"\r", "\x1b[2K", "\x1b[?25l"} {
		assert.NotContains(t, r.stdout.Raw(), sequence)
	}
}

func TestRichDeployShowsDurationOnlyFromTwoSeconds(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.moveClock(map[string]time.Duration{callUploadSource: 2400 * time.Millisecond, callValidate: 1900 * time.Millisecond})

	require.NoError(t, r.run())

	out := lines(r.stdout.Screen())
	assert.Regexp(t, `^✓ Source uploaded\s+\(2\.4s\)$`, lineWith(t, out, "Source uploaded"))
	assert.NotContains(t, lineWith(t, out, "Parameters validated"), "(")
	assert.NotContains(t, lineWith(t, out, "Package created"), "(")
}

func TestRichDeployBuildFramesUseTheGutter(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.frames = []string{"\x1b]0;title\a" + "building", "\x1b[35mℹ\x1b[0m Status - completed", "two\n"}

	require.NoError(t, r.run())

	frames := blockFrom(t, lines(r.stdout.Screen()), "  │ building")
	assert.Equal(t, []string{"  │ building", "  │ ℹ Status - completed", "  │ two", "  │"}, frames[:4])
	assert.Equal(t, "✓ Package built", frames[4])
	assert.Contains(t, r.stdout.Raw(), "\x1b[35mℹ\x1b[0m Status - completed", "the server's colour is untouched")
	assert.NotContains(t, r.stdout.Raw(), "\x1b]0;")
}

func TestRichDeployResultBlockIsLiteral(t *testing.T) {
	const title = "Instance has been deployed!"
	rows := func(hosts ...string) string {
		text := "\n  Instance id: i-1\n  Instance service name: svc-1\n"
		for _, host := range hosts {
			text += "  Instance host address: " + host + "\n"
		}
		return text
	}
	tests := []struct {
		name     string
		deployed platform.DeployResult
		want     string
	}{
		{
			name: "one row per address, with no colour in the rows",
			deployed: platform.DeployResult{
				InstanceID: "i-1", ServiceName: "svc-1", HostURLs: []string{"https://a.example.test", "https://b.example.test"},
			},
			want: rows("https://a.example.test", "https://b.example.test"),
		},
		{
			name:     "no address, no row",
			deployed: platform.DeployResult{InstanceID: "i-1", ServiceName: "svc-1"},
			want:     rows(),
		},
		{
			name: "control sequences in the platform's words are removed",
			deployed: platform.DeployResult{
				InstanceID: "i-\x1b[2J1", ServiceName: "svc-\x1b]0;title\a1", HostURLs: []string{"https://a.\x1b[31mexample.test"},
			},
			want: rows("https://a.example.test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRichRun(t, term.Rich)
			r.api.deployed = tt.deployed

			require.NoError(t, r.run())

			_, block, found := strings.Cut(r.stdout.Raw(), title)
			require.True(t, found)
			assert.Equal(t, tt.want, block)
		})
	}
}

func TestRichDeployCreatesAMissingProject(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.plan.ProjectName = "brand-new"
	r.api.getProjectErr = fmt.Errorf("project: %w", platform.ErrNotFound)
	r.api.newProjectID = "p-new"

	require.NoError(t, r.run())

	out := squashed(lines(r.stdout.Screen()))
	assert.Equal(t, "✓ Project created brand-new", out[0])
	assert.NotContains(t, r.stdout.Screen(), "Project resolved")
	assert.Equal(t, []string{
		callGetProject, callCreateProject, callValidate, callUploadSource, callCreatePackage, callWatchBuild,
		callDeployInstance,
	}, r.api.calls)
	assert.Equal(t, "brand-new", r.api.createdName)
	assert.Equal(t, "p-new", r.api.validateReq.ProjectID)
	assert.Equal(t, "p-new", r.api.deployReq.ProjectID)
}

func TestRichDeployTgzSkipsPackaging(t *testing.T) {
	r := newRichRun(t, term.Rich)
	tmp := tempDirHolder(t)
	r.plan.TgzPath = writeTgz(t, t.TempDir(), "packed.js")

	require.NoError(t, r.run())

	out := squashed(lines(r.stdout.Screen()))
	assert.Contains(t, out, "✓ Source uploaded")
	assert.NotContains(t, r.stdout.Screen(), "Files packaged")
	assert.Equal(t, r.plan.TgzPath, r.api.uploadedPath, "the path is used as typed")
	assert.Equal(t, []string{"packed.js"}, r.api.entries, "the project directory is not packaged")
	requireEmptyDir(t, tmp)
}

func TestRichDeployWarningsGoToStderr(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".git/config", "[core]\n")
	writeFile(t, dir, ".env", "A=1\n")
	writeFile(t, dir, "app.js", "console.log(1);\n")
	classic := newHarness(t)
	classic.plan.ProjectDir = dir
	require.NoError(t, classic.run())
	r := newRichRun(t, term.Rich)
	r.plan.ProjectDir = dir

	require.NoError(t, r.run())

	assert.True(t, strings.HasPrefix(r.stderr.Screen(), "! Skipped 1 file(s) under .git/ and 1 .env file(s)"))
	assert.Len(t, lines(r.stderr.Screen()), 1)
	assert.Equal(t, classic.stderr.String(), r.stderr.Screen(), "the notice is the classic one, byte for byte")
	assert.NotContains(t, r.stdout.Screen(), "Skipped")
	assert.Contains(t, squashed(lines(r.stdout.Screen())), "✓ Files packaged 1 file")
}

func TestFilesLabel(t *testing.T) {
	tests := []struct {
		files int
		want  string
	}{
		{0, "0 files"},
		{1, "1 file"},
		{2, "2 files"},
		{42, "42 files"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, filesLabel(tt.files))
		})
	}
}
