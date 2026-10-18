package initcmd

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
)

const templateManifest = `project:
  name: template-name
instance:
  name: template-instance
  runtime: nodejs16
  region: aws.use1
  application-id: ignored
  environment:
    - name: FROM_TEMPLATE
      value: "1"
  entrypoint: [node, index.js]
debug:
  name: debug
  application-id: ignored
  entrypoint: [node, --inspect, index.js]
unknown-key: dropped
`

// templateHarness picks the starter template, which the fake serves as archive.
func templateHarness(t *testing.T, archive []byte) *harness {
	t.Helper()
	h := newHarness(t, flowAsks(appOneLabel, "SKIP", pickStarter())...)
	h.api.archive = archive
	return h
}

func TestTemplateIsMergedOverTheAnswers(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("index.js", "console.log('hi');\n"), entry("vcr.yml", templateManifest)))

	require.NoError(t, h.run())

	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, &manifest.Manifest{
		Project: manifest.Project{Name: "my-project"},
		Instance: manifest.Instance{
			Name:          "dev",
			Runtime:       "nodejs18",
			Region:        "aws.euw1",
			ApplicationID: "a1",
			Environment:   []manifest.Env{{Name: "FROM_TEMPLATE", Value: "1"}},
			Entrypoint:    []string{"node", "index.js"},
		},
		Debug: manifest.Debug{
			Name:          "debug",
			ApplicationID: wantAppPlaceholder,
			Entrypoint:    []string{"node", "--inspect", "index.js"},
		},
	}, got)
	assert.NotContains(t, readText(t, h.path("vcr.yml")), "unknown-key")
	assert.Equal(t, "console.log('hi');\n", readText(t, h.path("index.js")))
	assert.Equal(t, "✓ "+h.path("vcr.yml")+" created\n", h.stdout.String())
	assert.Empty(t, h.stderr.String())
}

func TestTemplateManifestWithAnotherNameIsRenamed(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("index.js", "x\n"), entry("neru.yml", templateManifest)))

	require.NoError(t, h.run())

	assert.FileExists(t, h.path("vcr.yml"))
	assert.NoFileExists(t, h.path("neru.yml"))
	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, "dev", got.Instance.Name)
	assert.Equal(t, []string{"node", "index.js"}, got.Instance.Entrypoint)
	assert.Equal(t, "✓ "+h.path("vcr.yml")+" created\n", h.stdout.String())
}

func TestTemplateWithoutAManifestIsAnError(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("index.js", "x\n")))

	err := h.run()

	assert.EqualError(t, err, "failed to find template manifest file: manifest file not found")
	assert.FileExists(t, h.path("index.js"))
	assert.NoFileExists(t, h.path("vcr.yml"))
	assert.Empty(t, h.stdout.String())
}

func TestUnparsableTemplateManifestWarns(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("index.js", "x\n"), entry("vcr.yml", "project: [")))

	require.NoError(t, h.run())

	assert.Equal(t, "! Failed to parse template manifest file due to yaml: line 1: did not find expected node content.\n",
		h.stderr.String())
	assert.Equal(t, answersYAML("a1"), readText(t, h.path("vcr.yml")))
	assert.Equal(t, "✓ "+h.path("vcr.yml")+" created\n", h.stdout.String())
}

func TestUnparsableManifestUnderAnotherNameIsReplacedByVcrYml(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("neru.yaml", "project: [")))

	require.NoError(t, h.run())

	assert.Contains(t, h.stderr.String(), "! Failed to parse template manifest file due to ")
	assert.NoFileExists(t, h.path("neru.yaml"))
	assert.Equal(t, answersYAML("a1"), readText(t, h.path("vcr.yml")))
}

func TestParseWarningTextIsSanitised(t *testing.T) {
	// The parser quotes the offending value, here one that holds a colour sequence.
	h := templateHarness(t, tarGz(t, entry("vcr.yml", "project:\n  name: x\ninstance:\n  scaling:\n    min-scale: \"\\e[31mred\"\n")))

	require.NoError(t, h.run())

	assert.NotContains(t, h.stderr.String(), "\x1b")
	assert.Contains(t, h.stderr.String(), "cannot unmarshal !!str `red` into int")
	assert.True(t, strings.HasPrefix(h.stderr.String(), "! Failed to parse template manifest file due to "), h.stderr.String())
}

func TestNoTemplatesForTheRuntime(t *testing.T) {
	h := newHarness(t, flowAsks(appOneLabel, "SKIP")...)
	h.api.templates = []platform.Template{
		{ID: "p2", Name: "Starter Project - Python", ProgrammingLanguage: "python", ActiveVersionID: "v2"},
	}

	require.NoError(t, h.run())

	assert.Equal(t, "! No product templates available for the selected runtime \"nodejs18\"\n", h.stderr.String())
	assert.Equal(t, answersYAML("a1"), readText(t, h.path("vcr.yml")))
	assert.Zero(t, h.api.count(methodDownloadTemplate))
	assert.Equal(t, "✓ "+h.path("vcr.yml")+" created\n", h.stdout.String())
}

func TestTemplateWithoutAnActiveVersion(t *testing.T) {
	h := templateHarness(t, nil)
	h.api.templates = []platform.Template{{ID: "p1", Name: starterName, ProgrammingLanguage: "nodejs"}}

	err := h.run()

	assert.EqualError(t, err, "failed to get the active product template version: not found")
	assert.ErrorIs(t, err, platform.ErrNotFound)
	assert.Zero(t, h.api.count(methodDownloadTemplate))
	assert.NoFileExists(t, h.path("vcr.yml"))
}

func TestDownloadUsesTheSelectedIDs(t *testing.T) {
	h := newHarness(t, flowAsks("SKIP", "SKIP",
		choose(msgTemplate, "", "Voice App", "SKIP", starterName, "Voice App"))...)
	h.api.templates = []platform.Template{
		{ID: "p2", Name: "Voice App", ProgrammingLanguage: "nodejs", ActiveVersionID: "v2"},
		{ID: "p1", Name: starterName, ProgrammingLanguage: "nodejs", ActiveVersionID: "v1"},
		{ID: "p3", Name: "Python", ProgrammingLanguage: "python", ActiveVersionID: "v3"},
	}
	h.api.archive = tarGz(t, entry("vcr.yml", "project:\n  name: voice\n"))

	require.NoError(t, h.run())

	assert.Equal(t, []download{{product: "p2", version: "v2"}}, h.api.downloads)
}

func TestExtractionErrorIsWrapped(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("../x", "evil\n")))

	err := h.run()

	assert.EqualError(t, err,
		`failed to uncompress template files: template entry "../x" is outside the project or inside .git`)
	assert.NoFileExists(t, h.path("vcr.yml"))
	_, statErr := os.Stat(h.path("../x"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "nothing is written outside the project")
}

func TestTemplateCannotWriteIntoGit(t *testing.T) {
	h := templateHarness(t, tarGz(t, entry("vcr.yml", templateManifest), entry(".git/config", "[core]\n")))

	err := h.run()

	assert.EqualError(t, err,
		`failed to uncompress template files: template entry ".git/config" is outside the project or inside .git`)
	assert.NoDirExists(t, h.path(".git"))
}

func TestInvalidArchiveIsWrapped(t *testing.T) {
	h := templateHarness(t, []byte("not an archive"))

	err := h.run()

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to uncompress template files: failed to create a new gzip reader: "), err.Error())
}

func TestRuntimeNameInThePromptCarriesNoControlSequences(t *testing.T) {
	const rawName = "node\x1b[31mjs\x1b[0m"
	asks := flowAsks("SKIP", "SKIP", choose("Select a product template for runtime nodejs: ", "", "SKIP", "SKIP", starterName))
	asks[2] = choose(msgRuntime, "", "nodejs", "nodejs")
	h := newHarness(t, asks...)
	h.api.runtimes = []platform.Runtime{{Name: rawName, Language: "nodejs"}}

	require.NoError(t, h.run())

	got, err := manifest.Read(h.path("vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, rawName, got.Instance.Runtime, "the manifest names the runtime as the platform does")
}
