package initcmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestRichInitResolvesLookupsIntoSteps(t *testing.T) {
	h := newModeHarness(t, term.Rich, flowAsks(appOneLabel, "SKIP", pickStarter())...)
	h.serveTemplate()

	require.NoError(t, h.run("my-project"))

	assert.Equal(t, []string{
		"✓ Runtimes retrieved",
		"✓ Regions retrieved",
		"✓ Applications retrieved",
		"✓ Applications retrieved",
		"✓ Templates retrieved",
		"✓ Template downloaded",
		"✓ Template unpacked",
		"✓ Project created in " + filepath.Join(h.base, "my-project"),
		"Next: cd my-project && vcr deploy",
	}, lines(h.stdout()))
	for _, classic := range []string{"vcr.yml created", "Retrieving", "Downloading", "Uncompressing"} {
		assert.NotContains(t, h.rich.Raw(), classic)
	}
	assert.Empty(t, h.stderr())
	noLiveBytes(t, h.rich.Raw(), h.richErr.Raw())
}

func TestRichInitWithoutATemplateSkipsTheDownloadSteps(t *testing.T) {
	h := newModeHarness(t, term.Rich, flowAsks(appOneLabel, "SKIP", skipTemplate())...)

	require.NoError(t, h.run("my-project"))

	assert.Equal(t, []string{
		"✓ Runtimes retrieved",
		"✓ Regions retrieved",
		"✓ Applications retrieved",
		"✓ Applications retrieved",
		"✓ Templates retrieved",
		"✓ Project created in " + filepath.Join(h.base, "my-project"),
		"Next: cd my-project && vcr deploy",
	}, lines(h.stdout()))
	assert.Zero(t, h.api.count(methodDownloadTemplate))
}

func TestRichInitNoTemplatesWarningKeepsClassicText(t *testing.T) {
	h := newModeHarness(t, term.Rich, flowAsks(appOneLabel, "SKIP")...)
	h.api.templates = []platform.Template{
		{ID: "p2", Name: "Starter Project - Python", ProgrammingLanguage: "python", ActiveVersionID: "v2"},
	}

	require.NoError(t, h.run("my-project"))

	assert.Equal(t, "! No product templates available for the selected runtime \"nodejs18\"\n", h.stderr())
	assert.Contains(t, lines(h.stdout()), "✓ Templates retrieved")
	assert.NotContains(t, h.stdout(), "Template downloaded")
	assert.NotContains(t, h.stdout(), "Template unpacked")
}

func TestRichInitNewApplicationStep(t *testing.T) {
	asks := append(head(),
		appAsk(msgDeployApp, "CREATE NEW APP"),
		input(msgNewDeployApp, "", "my-new-app"),
		input(msgNewDeployApp, "", "my-new-app"),
		appAsk(msgDebugApp, "SKIP", "my-new-app - (id-of-my-new-app)"),
		skipTemplate(),
	)
	h := newModeHarness(t, term.Rich, asks...)
	h.api.createErrs = []error{errBoom}

	require.NoError(t, h.run("my-project"))

	assert.Equal(t, "✗ boom\n", h.stderr())
	assert.Contains(t, squashed(lines(h.stdout())), "✓ Application created my-new-app")
	assert.Equal(t, 2, h.rec.count(kindInput, msgNewDeployApp), "the name is asked again after the failure")
	assert.Equal(t, []createdApp{{name: "my-new-app"}, {name: "my-new-app"}}, h.api.created)
}

func TestRichInitNextStepForTheWorkingDirectory(t *testing.T) {
	h := newModeHarness(t, term.Rich, flowAsks(appOneLabel, "SKIP", skipTemplate())...)
	dir := filepath.Join(h.base, "my-project")
	require.NoError(t, os.Mkdir(dir, 0o750))
	h.cwd = dir

	require.NoError(t, h.run(""))

	out := lines(h.stdout())
	assert.Equal(t, "✓ Project created in "+dir, out[len(out)-2])
	assert.Equal(t, "Next: vcr deploy", out[len(out)-1])
	assert.FileExists(t, filepath.Join(dir, "vcr.yml"))
}

func TestRichInitFailuresMatchClassic(t *testing.T) {
	start := []string{"✓ Runtimes retrieved", "✓ Regions retrieved", "✓ Applications retrieved", "✓ Applications retrieved"}
	tests := []struct {
		name    string
		asks    int // questions answered before the failing call
		setup   func(*fakeAPI)
		archive []byte
		shown   []string // the resolved lines the rich screen holds
	}{
		{
			name:  "runtimes",
			asks:  2,
			setup: func(f *fakeAPI) { f.failures = map[string]error{methodListRuntimes: errBoom} },
			shown: []string{},
		},
		{
			name:  "regions",
			asks:  3,
			setup: func(f *fakeAPI) { f.failures = map[string]error{methodListRegions: errBoom} },
			shown: start[:1],
		},
		{
			name:  "deployment applications",
			asks:  4,
			setup: func(f *fakeAPI) { f.listAppsErrs = []error{errBoom} },
			shown: start[:2],
		},
		{
			name:  "debug applications",
			asks:  5,
			setup: func(f *fakeAPI) { f.listAppsErrs = []error{nil, errBoom} },
			shown: start[:3],
		},
		{
			name:  "templates",
			asks:  6,
			setup: func(f *fakeAPI) { f.failures = map[string]error{methodListTemplates: errBoom} },
			shown: start,
		},
		{
			name:  "download",
			asks:  7,
			setup: func(f *fakeAPI) { f.failures = map[string]error{methodDownloadTemplate: errBoom} },
			shown: append(slices.Clone(start), "✓ Templates retrieved"),
		},
		{
			name:    "extraction of an entry outside the project",
			asks:    7,
			archive: tarGz(t, entry("../x", "evil\n")),
			shown:   append(slices.Clone(start), "✓ Templates retrieved", "✓ Template downloaded"),
		},
		{
			name:    "a template without a manifest",
			asks:    7,
			archive: tarGz(t, entry("index.js", "x\n")),
			shown: append(slices.Clone(start), "✓ Templates retrieved", "✓ Template downloaded",
				"✓ Template unpacked"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asks := flowAsks("SKIP", "SKIP", pickStarter())[:tt.asks]
			classic := newModeHarness(t, term.Classic, asks...)
			rich := newModeHarness(t, term.Rich, asks...)
			for _, h := range []*modeHarness{classic, rich} {
				if tt.setup != nil {
					tt.setup(h.api)
				}
				h.api.archive = tt.archive
			}

			classicErr := classic.run("my-project")
			richErr := rich.run("my-project")

			require.Error(t, classicErr)
			require.Error(t, richErr)
			assert.Equal(t, classicErr.Error(), richErr.Error())
			assert.Equal(t, classic.apiCalls(), rich.apiCalls())
			assert.NotContains(t, rich.stdout(), "✗")
			assert.NotContains(t, rich.stderr(), "✗", "a failed step is erased, not printed as a failure")
			assert.NotContains(t, rich.stdout(), "Project created")
			assert.Equal(t, tt.shown, lines(rich.stdout()), "the steps that finished, and none for the one that failed")
		})
	}
}

func TestRichInitCallsMatchClassic(t *testing.T) {
	asks := flowAsks(appOneLabel, "SKIP", pickStarter())
	classic := newModeHarness(t, term.Classic, asks...)
	rich := newModeHarness(t, term.Rich, asks...)
	classic.serveTemplate()
	rich.serveTemplate()

	require.NoError(t, classic.run("my-project"))
	require.NoError(t, rich.run("my-project"))

	assert.Equal(t, classic.apiCalls(), rich.apiCalls())
	assert.Equal(t, classic.rec.asked, rich.rec.asked, "the questions are the same, with the same defaults and options")
	assert.Equal(t, readText(t, classic.project("vcr.yml")), readText(t, rich.project("vcr.yml")))
	assert.Equal(t, readText(t, classic.project("index.js")), readText(t, rich.project("index.js")))
}

func TestStaticInitMatchesRichScreen(t *testing.T) {
	asks := flowAsks(appOneLabel, "SKIP", pickStarter())
	rich := newModeHarness(t, term.Rich, asks...)
	static := newModeHarness(t, term.Static, asks...)
	rich.serveTemplate()
	static.serveTemplate()

	require.NoError(t, rich.run("my-project"))
	require.NoError(t, static.run("my-project"))

	want := strings.ReplaceAll(rich.stdout(), rich.base, static.base)
	assert.Equal(t, want, static.stdout())
	assert.Equal(t, rich.stderr(), static.stderr())
	noLiveBytes(t, static.rich.Raw(), static.richErr.Raw())
}

func TestRichInitAgainstTheFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	client := fakeClient(t, srv)
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(base)
	streams, stdout, stderr := term.TestRich(term.Rich, 100, clocktest.New(richStart))

	err = run(t.Context(), deps{
		IO: streams, Prompt: newScript(t, fakeFlow()...), API: client, DefaultRegion: "aws.euw1",
	}, "my-project")

	require.NoError(t, err)
	out := lines(stdout.Screen())
	require.GreaterOrEqual(t, len(out), 2)
	assert.Equal(t, []string{
		"✓ Project created in " + filepath.Join(base, "my-project"),
		"Next: cd my-project && vcr deploy",
	}, out[len(out)-2:])
	assert.Empty(t, stderr.Raw())
	assert.FileExists(t, filepath.Join(base, "my-project", "vcr.yml"))

	var rest, market []string
	for _, r := range srv.Requests() {
		switch r.Kind {
		case "rest":
			rest = append(rest, r.Method+" "+r.Rel)
		case "market":
			market = append(market, r.Method+" "+requestID.ReplaceAllString(r.Rel, "<id>"))
		default:
			t.Errorf("unexpected %s request %s %s", r.Kind, r.Method, r.Rel)
		}
	}
	assert.Equal(t, []string{
		"GET /runtimes", "GET /regions", "GET /applications", "GET /applications", "GET /products/templates",
	}, rest)
	assert.Equal(t, []string{"GET /products/<id>/versions/<id>/download/source"}, market)
}
