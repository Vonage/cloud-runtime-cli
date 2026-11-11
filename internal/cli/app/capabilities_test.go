package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/app"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const baseManifest = `# the project
project:
  name: my-project
instance:
  name: dev
  runtime: nodejs22
custom: 1
`

const customVoice = `application-capabilities:
  voice:
    webhooks:
      answer_url:
        address: "{{instance_url}}/my-answer"
`

// capsHarness is a harness that needs no platform, no credentials and no config file, in a fresh project directory
// that is the working directory.
func capsHarness(t *testing.T) (*cmdtest.Harness, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	h := cmdtest.New(t, startFake(t), cmdtest.Options{NoPlatform: true, NoCredentials: true,
		Flags: configFlagsWithMissingFile(home)})
	dir := t.TempDir()
	t.Chdir(dir)
	wd, err := os.Getwd()
	require.NoError(t, err)
	return h, wd
}

// configFlagsWithMissingFile points the config file at a path that does not exist.
func configFlagsWithMissingFile(home string) config.Flags {
	return config.Flags{ConfigFile: filepath.Join(home, "missing", ".vcr-cli")}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// block returns the application-capabilities mapping of a YAML document.
func block(t *testing.T, doc string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(doc), &m))
	b, ok := m["application-capabilities"].(map[string]any)
	require.True(t, ok, "no application-capabilities mapping in:\n%s", doc)
	return b
}

// normalized is the default of a capability as it reads back from YAML.
func normalized(t *testing.T, name string, n manifest.NetworkAPIs) any {
	t.Helper()
	def, err := manifest.DefaultCapability(name, n)
	require.NoError(t, err)
	data, err := yaml.Marshal(def)
	require.NoError(t, err)
	var v any
	require.NoError(t, yaml.Unmarshal(data, &v))
	return v
}

func TestCapabilitiesGroupIsAnnotatedNoneAndHasAdd(t *testing.T) {
	h, _ := capsHarness(t)

	group, _, err := app.NewCmd(h.Env).Find([]string{"capabilities"})

	require.NoError(t, err)
	assert.Equal(t, "capabilities", group.Name())
	assert.Equal(t, cmdenv.None, group.Annotations[cmdenv.Annotation])
	assert.False(t, group.Runnable())
	require.Len(t, group.Commands(), 1)
	add := group.Commands()[0]
	assert.Equal(t, "add", add.Name())
	assert.Empty(t, add.Annotations[cmdenv.Annotation], "add takes its mode from the group")
}

func TestCapabilitiesAddWritesTheDefaultOfEachFlag(t *testing.T) {
	net := manifest.NetworkAPIs{ApplicationID: "net-1", RedirectURI: "https://example.com/cb"}
	for _, name := range manifest.CapabilityNames {
		t.Run(name, func(t *testing.T) {
			h, dir := capsHarness(t)
			path := filepath.Join(dir, "vcr.yml")
			writeFile(t, path, baseManifest)
			args := []string{"capabilities", "add", "--" + strings.ReplaceAll(name, "_", "-")}
			n := manifest.NetworkAPIs{}
			if name == "network_apis" {
				args = append(args, "--network-application-id", net.ApplicationID, "--redirect-uri", net.RedirectURI)
				n = net
			}

			err := h.Run(t.Context(), app.NewCmd, args...)

			require.NoError(t, err)
			written := readFile(t, path)
			assert.Equal(t, map[string]any{name: normalized(t, name, n)}, block(t, written))
			assert.True(t, strings.HasPrefix(written, baseManifest), "the rest of the file is kept:\n%s", written)
			assert.Equal(t, h.Env.IO.Style().SuccessIcon()+" Added "+name+" to application-capabilities in "+path+"\n",
				h.Stderr.String())
			assert.Empty(t, h.Stdout.String())
			assert.Empty(t, h.Requests())
		})
	}
}

func TestCapabilitiesAddFlagErrorsLeaveTheFileAlone(t *testing.T) {
	const (
		noFlag    = "no capability given: pass one or more of --voice, --messages, --rtc, --verify, --video, --vbc, --meetings, --network-apis"
		missing   = "--network-apis needs --network-application-id and --redirect-uri"
		withoutNA = "--network-application-id and --redirect-uri need --network-apis"
	)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no capability flag", nil, noFlag},
		{"no capability flag but --force", []string{"--force"}, noFlag},
		{"network apis without the application id", []string{"--network-apis", "--redirect-uri", "https://example.com/cb"}, missing},
		{"network apis without the redirect uri", []string{"--network-apis", "--network-application-id", "net-1"}, missing},
		{"network apis without either value", []string{"--network-apis"}, missing},
		{"application id without network apis", []string{"--voice", "--network-application-id", "net-1"}, withoutNA},
		{"redirect uri without network apis", []string{"--voice", "--redirect-uri", "https://example.com/cb"}, withoutNA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, dir := capsHarness(t)
			path := filepath.Join(dir, "vcr.yml")
			writeFile(t, path, baseManifest)

			err := h.Run(t.Context(), app.NewCmd, append([]string{"capabilities", "add"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Equal(t, baseManifest, readFile(t, path))
			assert.Empty(t, h.Stderr.String())
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestCapabilitiesAddSkipsAnExistingCapability(t *testing.T) {
	h, dir := capsHarness(t)
	path := filepath.Join(dir, "vcr.yml")
	writeFile(t, path, baseManifest+customVoice)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice", "--messages")

	require.NoError(t, err)
	got := block(t, readFile(t, path))
	assert.Equal(t, block(t, customVoice)["voice"], got["voice"])
	assert.Equal(t, normalized(t, "messages", manifest.NetworkAPIs{}), got["messages"])
	style := h.Env.IO.Style()
	assert.Equal(t,
		style.WarningIcon()+" voice is already configured, skipped (use --force to replace it)\n"+
			style.SuccessIcon()+" Added messages to application-capabilities in "+path+"\n",
		h.Stderr.String())
}

func TestCapabilitiesAddForceReplacesOnlyTheNamedCapability(t *testing.T) {
	h, dir := capsHarness(t)
	path := filepath.Join(dir, "vcr.yml")
	rtc := "  rtc:\n    webhooks:\n      event_url:\n        address: \"{{instance_url}}/my-rtc\"\n"
	writeFile(t, path, baseManifest+customVoice+rtc)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice", "--force")

	require.NoError(t, err)
	got := block(t, readFile(t, path))
	assert.Equal(t, normalized(t, "voice", manifest.NetworkAPIs{}), got["voice"])
	assert.Equal(t, block(t, customVoice+rtc)["rtc"], got["rtc"])
	assert.Equal(t, h.Env.IO.Style().SuccessIcon()+" Replaced voice in application-capabilities in "+path+"\n",
		h.Stderr.String())
}

func TestCapabilitiesAddRemovesTheCapabilitiesList(t *testing.T) {
	h, dir := capsHarness(t)
	path := filepath.Join(dir, "vcr.yml")
	writeFile(t, path, "instance:\n  name: dev\n  capabilities: [voice, messages-v1]\n")

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice", "--messages")

	require.NoError(t, err)
	written := readFile(t, path)
	assert.NotContains(t, written, "messages-v1")
	assert.Len(t, block(t, written), 2)
	style := h.Env.IO.Style()
	assert.Equal(t,
		style.SuccessIcon()+" Added voice to application-capabilities in "+path+"\n"+
			style.SuccessIcon()+" Added messages to application-capabilities in "+path+"\n"+
			style.WarningIcon()+" Removed instance.capabilities (voice, messages-v1): application-capabilities replaces it\n",
		h.Stderr.String())
}

func TestCapabilitiesAddDoesNotRewriteAnUnchangedFile(t *testing.T) {
	h, dir := capsHarness(t)
	path := filepath.Join(dir, "vcr.yml")
	writeFile(t, path, baseManifest+customVoice)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, old, old))

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice")

	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "the file was rewritten")
	assert.Equal(t, h.Env.IO.Style().WarningIcon()+" voice is already configured, skipped (use --force to replace it)\n",
		h.Stderr.String())
}

func TestCapabilitiesAddWritesTheFilenameFlag(t *testing.T) {
	h, dir := capsHarness(t)
	inDir := filepath.Join(dir, "vcr.yml")
	writeFile(t, inDir, baseManifest)
	other := filepath.Join(dir, "other", "custom.yml")
	writeFile(t, other, baseManifest)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--rtc", "-f", other)

	require.NoError(t, err)
	assert.Equal(t, baseManifest, readFile(t, inDir))
	assert.Contains(t, block(t, readFile(t, other)), "rtc")
	assert.Equal(t, h.Env.IO.Style().SuccessIcon()+" Added rtc to application-capabilities in "+other+"\n",
		h.Stderr.String())
}

func TestCapabilitiesAddWithoutManifestFailsAndCreatesNothing(t *testing.T) {
	h, dir := capsHarness(t)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice")

	require.EqualError(t, err,
		"failed to find template manifest file: manifest file not found (use --print to see the block without a manifest)")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCapabilitiesAddPrintWithoutManifest(t *testing.T) {
	h, dir := capsHarness(t)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--rtc", "--print")

	require.NoError(t, err)
	out := h.Stdout.String()
	assert.True(t, strings.HasPrefix(out, "application-capabilities:\n  rtc:\n"), "stdout:\n%s", out)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))
	assert.Len(t, doc, 1)
	assert.Equal(t, map[string]any{"rtc": normalized(t, "rtc", manifest.NetworkAPIs{})}, block(t, out))
	assert.Empty(t, h.Stderr.String())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Empty(t, h.Requests())
}

func TestCapabilitiesAddPrintMergesWithTheManifestAndWritesNothing(t *testing.T) {
	h, dir := capsHarness(t)
	path := filepath.Join(dir, "vcr.yml")
	writeFile(t, path, baseManifest+customVoice)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--voice", "--rtc", "--print")

	require.NoError(t, err)
	out := h.Stdout.String()
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))
	assert.Len(t, doc, 1, "stdout holds only the block:\n%s", out)
	got := block(t, out)
	assert.Equal(t, block(t, customVoice)["voice"], got["voice"])
	assert.Equal(t, normalized(t, "rtc", manifest.NetworkAPIs{}), got["rtc"])
	assert.Equal(t, h.Env.IO.Style().WarningIcon()+" voice is already configured, skipped (use --force to replace it)\n",
		h.Stderr.String())
	assert.Equal(t, baseManifest+customVoice, readFile(t, path))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestCapabilitiesAddNeedsNoConfigFileAndSendsNoRequest(t *testing.T) {
	h, dir := capsHarness(t)
	writeFile(t, filepath.Join(dir, "vcr.yml"), baseManifest)

	err := h.Run(t.Context(), app.NewCmd, "capabilities", "add", "--vbc")

	require.NoError(t, err)
	assert.Empty(t, h.Requests())
	_, statErr := os.Stat(h.Env.Flags.ConfigFile)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no config file is created")
}
