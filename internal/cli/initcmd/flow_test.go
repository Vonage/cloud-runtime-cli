package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const (
	msgProjectName  = "Enter your project name:"
	msgInstanceName = "Enter your Instance name:"
	msgRuntime      = "Select a runtime:"
	msgRegion       = "Select a region:"
	msgDeployApp    = "Select your Vonage application ID for deployment:"
	msgDebugApp     = "Select your Vonage application ID for debug:"
	msgNewDeployApp = "Enter your new Vonage application name for deployment:"
	msgNewDebugApp  = "Enter your new Vonage application name for debug:"
	msgTemplate     = "Select a product template for runtime nodejs18: "

	regionLabel = "AWS Europe (Ireland) - (aws.euw1)"
	appOneLabel = "App One - (a1)"
	starterName = "Starter Project - Node.js"

	wantAppPlaceholder        = "<please create an application on VONAGE APIs dashboard>"
	wantEntrypointPlaceholder = "<please include the entrypoint for your script>"
)

// head are the first four questions of a run that takes every default.
func head() []ask {
	return []ask{
		input(msgProjectName, "my-project", "my-project"),
		input(msgInstanceName, "dev", "dev"),
		choose(msgRuntime, "nodejs18", "nodejs18", "nodejs18", "nodejs22 - (latest)"),
		choose(msgRegion, regionLabel, regionLabel, regionLabel),
	}
}

// appAsk is an application question; created are the applications that exist besides App One.
func appAsk(message, answer string, created ...string) ask {
	return choose(message, "", answer, append([]string{"SKIP", "CREATE NEW APP", appOneLabel}, created...)...)
}

// flowAsks are the questions of a run that picks the given applications and then asks rest.
func flowAsks(deployApp, debugApp string, rest ...ask) []ask {
	asks := append(head(), appAsk(msgDeployApp, deployApp), appAsk(msgDebugApp, debugApp))
	return append(asks, rest...)
}

func skipTemplate() ask {
	return choose(msgTemplate, "", "SKIP", "SKIP", starterName)
}

func pickStarter() ask {
	return choose(msgTemplate, "", starterName, "SKIP", starterName)
}

// answersYAML is the manifest init writes when no template is used.
func answersYAML(deployApp string) string {
	return "project:\n    name: my-project\n" +
		"instance:\n    name: dev\n    runtime: nodejs18\n    region: aws.euw1\n    application-id: " + deployApp + "\n" +
		"    entrypoint:\n        - " + wantEntrypointPlaceholder + "\n" +
		"debug:\n    application-id: " + wantAppPlaceholder + "\n" +
		"    entrypoint:\n        - " + wantEntrypointPlaceholder + "\n"
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestFullFlowWithoutATemplate(t *testing.T) {
	h := newHarness(t, flowAsks(appOneLabel, "SKIP", skipTemplate())...)

	require.NoError(t, h.run())

	assert.Equal(t, answersYAML("a1"), readText(t, h.path("vcr.yml")))
	assert.Equal(t, "✓ "+h.path("vcr.yml")+" created\n", h.stdout.String())
	assert.Empty(t, h.stderr.String())
	assert.Equal(t, []string{
		methodListRuntimes, methodListRegions, methodListApplications, methodListApplications, methodListTemplates,
	}, h.api.calls)
	assert.Equal(t, []string{"", ""}, h.api.filters, "both application lists are unfiltered")
}

func TestSkippedApplicationsKeepThePlaceholder(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", "SKIP", skipTemplate())...)

	require.NoError(t, h.run())

	assert.Equal(t, answersYAML(wantAppPlaceholder), readText(t, h.path("vcr.yml")))
}

func TestChosenDebugApplicationIsWrittenUnderDebug(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", appOneLabel, skipTemplate())...)

	require.NoError(t, h.run())

	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, wantAppPlaceholder, got.Instance.ApplicationID)
	assert.Equal(t, "a1", got.Debug.ApplicationID)
}

func TestSpinnerLabels(t *testing.T) {
	h := newHarness(t, append(head(),
		appAsk(msgDeployApp, "CREATE NEW APP"),
		input(msgNewDeployApp, "", "fresh"),
		appAsk(msgDebugApp, "SKIP", "fresh - (id-of-fresh)"),
		pickStarter(),
	)...)
	h.api.archive = tarGz(t, entry("vcr.yml", "project:\n  name: starter\n"))

	require.NoError(t, h.run())

	assert.Equal(t, []string{
		" Retrieving runtime list... ",
		" Retrieving region list... ",
		" Retrieving app list... ",
		` Creating Application "fresh"...`,
		" Retrieving app list... ",
		" Retrieving product templates... ",
		" Downloading template files... ",
		" Uncompressing template files... ",
	}, h.spin.labels)
	assert.Equal(t, len(h.spin.labels), h.spin.stopped, "every spinner is stopped")
}

func TestProjectNameLoopKeepsTheLastAnswerAsDefault(t *testing.T) {
	h := newHarness(t,
		input(msgProjectName, "my-project", "Bad_Name"),
		input(msgProjectName, "Bad_Name", "-x"),
		input(msgProjectName, "-x", "ok-name"),
		input(msgInstanceName, "dev", "").failing(errStop),
	)

	err := h.run()

	assert.EqualError(t, err, "failed to ask instance name: stop here")
	assert.Equal(t,
		`X project name "Bad_Name" is not correct,  project name should be lower case alphanumeric or - characters`+"\n"+
			`X project name "-x" is not correct,  project name should be lower case alphanumeric or - characters`+"\n",
		h.stderr.String())
	assert.Empty(t, h.stdout.String())
}

func TestTheProjectNameIsWhatWasAccepted(t *testing.T) {
	asks := flowAsks("SKIP", "SKIP", skipTemplate())
	asks[0] = input(msgProjectName, "my-project", "other-name")
	h := newHarness(t, asks...)

	require.NoError(t, h.run())

	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, "other-name", got.Project.Name)
}

func TestCreateNewAppRetriesAfterAnError(t *testing.T) {
	h := newHarness(t, append(head(),
		appAsk(msgDeployApp, "CREATE NEW APP"),
		input(msgNewDeployApp, "", "fresh"),
		input(msgNewDeployApp, "", "fresh"),
		appAsk(msgDebugApp, "CREATE NEW APP", "fresh - (id-of-fresh)"),
		input(msgNewDebugApp, "", "debugger"),
		skipTemplate(),
	)...)
	h.api.createErrs = []error{errBoom}

	require.NoError(t, h.run())

	assert.Equal(t, "X boom\n", h.stderr.String())
	assert.Equal(t, []createdApp{{name: "fresh"}, {name: "fresh"}, {name: "debugger"}}, h.api.created,
		"every call left the three capability flags off")
	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, "id-of-fresh", got.Instance.ApplicationID)
	assert.Equal(t, "id-of-debugger", got.Debug.ApplicationID)
}

func TestCreateApplicationErrorTextIsSanitised(t *testing.T) {
	h := newHarness(t, append(head(),
		appAsk(msgDeployApp, "CREATE NEW APP"),
		input(msgNewDeployApp, "", "fresh"),
		input(msgNewDeployApp, "", "fresh"),
		appAsk(msgDebugApp, "SKIP", "fresh - (id-of-fresh)"),
		skipTemplate(),
	)...)
	h.api.createErrs = []error{&textError{text: dirty}}

	require.NoError(t, h.run())

	assert.Equal(t, "X red\n", h.stderr.String())
}

func TestExistingManifestIsOverwrittenWithoutAsking(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", "SKIP", skipTemplate())...)
	require.NoError(t, os.MkdirAll(h.dir, 0o755))
	require.NoError(t, os.WriteFile(h.path("vcr.yml"), []byte("project:\n  name: old\n"), 0o600))

	require.NoError(t, h.run())

	assert.Equal(t, answersYAML(wantAppPlaceholder), readText(t, h.path("vcr.yml")))
}

func TestErrorsAreWrappedPerQuestion(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeAPI)
		asks  int // questions answered before the failing call
		want  string
	}{
		{"runtimes", func(f *fakeAPI) { f.failures = map[string]error{methodListRuntimes: errBoom} }, 2, "failed to ask runtime: boom"},
		{"regions", func(f *fakeAPI) { f.failures = map[string]error{methodListRegions: errBoom} }, 3, "failed to ask region: boom"},
		{"deployment applications", func(f *fakeAPI) { f.listAppsErrs = []error{errBoom} }, 4, "failed to ask instance app id: boom"},
		{"debug applications", func(f *fakeAPI) { f.listAppsErrs = []error{nil, errBoom} }, 5, "failed to ask debug app id: boom"},
		{"templates", func(f *fakeAPI) { f.failures = map[string]error{methodListTemplates: errBoom} }, 6, "failed to list product templates: boom"},
		{"download", func(f *fakeAPI) { f.failures = map[string]error{methodDownloadTemplate: errBoom} }, 7, "failed to download template files: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, flowAsks("SKIP", "SKIP", pickStarter())[:tt.asks]...)
			tt.setup(h.api)

			err := h.run()

			assert.EqualError(t, err, tt.want)
			assert.NoFileExists(t, h.path("vcr.yml"))
			assert.Empty(t, h.stdout.String())
		})
	}
}

func TestPromptErrorsAreWrapped(t *testing.T) {
	tests := []struct {
		asks int // the failing question is the last of this many
		want string
	}{
		{1, "failed to ask project name: boom"},
		{2, "failed to ask instance name: boom"},
		{3, "failed to ask runtime: boom"},
		{4, "failed to ask region: boom"},
		{5, "failed to ask instance app id: boom"},
		{6, "failed to ask debug app id: boom"},
		{7, "failed to ask user to select a product template for runtime nodejs18: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			asks := flowAsks("SKIP", "SKIP", pickStarter())[:tt.asks]
			asks[tt.asks-1] = asks[tt.asks-1].failing(errBoom)
			h := newHarness(t, asks...)

			err := h.run()

			assert.EqualError(t, err, tt.want)
			assert.NoFileExists(t, h.path("vcr.yml"))
		})
	}
}

func TestNewApplicationNameErrorIsWrapped(t *testing.T) {
	h := newHarness(t, append(head(), appAsk(msgDeployApp, "CREATE NEW APP"), input(msgNewDeployApp, "", "").failing(errBoom))...)

	err := h.run()

	assert.EqualError(t, err, "failed to ask instance app id: boom")
}

func TestEmptyRuntimeListStillReachesThePrompter(t *testing.T) {
	h := newHarness(t, input(msgProjectName, "my-project", "my-project"), input(msgInstanceName, "dev", "dev"),
		ask{kind: kindSelect, message: msgRuntime, err: errBoom})
	h.api.runtimes = nil

	err := h.run()

	assert.EqualError(t, err, "failed to ask runtime: boom")
	require.Len(t, h.prompt.offered, 1)
	assert.Empty(t, h.prompt.offered[0], "the prompter is the one that refuses an empty list")
}

func TestDirectoryIsCreatedBeforeTheFirstQuestion(t *testing.T) {
	h := newHarness(t, input(msgProjectName, "my-project", "").failing(errStop))

	err := h.run()

	assert.EqualError(t, err, "failed to ask project name: stop here")
	info, statErr := os.Stat(h.dir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Empty(t, h.api.calls, "nothing is asked of the platform before the first answer")
}

func TestNothingIsWrittenOutsideTheProjectOnAFailure(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", "SKIP")[:3]...)
	h.api.failures = map[string]error{methodListRegions: errBoom}

	require.Error(t, h.run())

	entries, err := os.ReadDir(filepath.Dir(h.dir))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the project directory")
	inside, err := os.ReadDir(h.dir)
	require.NoError(t, err)
	assert.Empty(t, inside)
}

// plantLink makes the project directory and puts a link named name in it that leads to a file outside it.
func plantLink(t *testing.T, h *harness, name string) (victim string) {
	t.Helper()
	victim = filepath.Join(t.TempDir(), "victim")
	require.NoError(t, os.WriteFile(victim, []byte("original\n"), 0o600))
	require.NoError(t, os.MkdirAll(h.dir, 0o755))
	if err := os.Symlink(victim, h.path(name)); err != nil {
		t.Skipf("links are not available here: %v", err)
	}
	return victim
}

func TestManifestIsNotWrittenThroughAPlantedLink(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", "SKIP", skipTemplate())...)
	victim := plantLink(t, h, "vcr.yml")

	err := h.run()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write manifest to ")
	assert.Equal(t, "original\n", readText(t, victim))
	assert.Empty(t, h.stdout.String(), "nothing is reported as created")
}

func TestTemplateManifestLookupStopsAtAPlantedLink(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("neru.yml", templateManifest)))
	victim := plantLink(t, h, "vcr.yml")

	err := h.run()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find template manifest file: ")
	assert.Equal(t, "original\n", readText(t, victim))
	assert.Empty(t, h.stdout.String(), "nothing is reported as created")
}
