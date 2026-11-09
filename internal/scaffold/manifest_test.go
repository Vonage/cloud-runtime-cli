package scaffold_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

const vcrYml = "vcr.yml"

// manifestNamed returns the YAML of a manifest whose project name is name, so a test can tell which file was read.
func manifestNamed(name string) string {
	return "project:\n  name: " + name + "\ninstance:\n  name: dev\n  runtime: nodejs22\n"
}

// writeManifestFile writes content as a manifest file and sets its modification time.
func writeManifestFile(t *testing.T, dir, name, content string, modified time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	write(t, content, path)
	require.NoError(t, os.Chtimes(path, modified, modified))
}

func sampleManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{Name: "my-project"},
		Instance: manifest.Instance{
			Name:          "dev",
			Runtime:       "nodejs22",
			Region:        "aws.euw1",
			ApplicationID: "11111111-2222-3333-4444-555555555555",
			Entrypoint:    []string{"node", "index.js"},
		},
	}
}

func marshalled(t *testing.T, m *manifest.Manifest) []byte {
	t.Helper()
	data, err := manifest.Marshal(m)
	require.NoError(t, err)
	return data
}

func TestReadManifestPicksTheNewestName(t *testing.T) {
	t.Parallel()
	base := time.Now().Add(-24 * time.Hour).Truncate(time.Second)

	t.Run("the latest modification time wins", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		writeManifestFile(t, dir, "neru.yaml", manifestNamed("neru.yaml"), base)
		writeManifestFile(t, dir, "neru.yml", manifestNamed("neru.yml"), base.Add(2*time.Hour))
		writeManifestFile(t, dir, "vcr.yaml", manifestNamed("vcr.yaml"), base.Add(time.Hour))
		writeManifestFile(t, dir, vcrYml, manifestNamed(vcrYml), base)

		m, name, ok, err := p.ReadManifest()

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "neru.yml", name)
		assert.Equal(t, "neru.yml", m.Project.Name)

		writeManifestFile(t, dir, vcrYml, manifestNamed(vcrYml), base.Add(3*time.Hour))

		m, name, ok, err = p.ReadManifest()

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, vcrYml, name)
		assert.Equal(t, vcrYml, m.Project.Name)
	})

	t.Run("a tie among all four names keeps the earliest", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		for _, name := range manifest.Names {
			writeManifestFile(t, dir, name, manifestNamed(name), base)
		}

		_, name, ok, err := p.ReadManifest()

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "neru.yaml", name)
	})

	t.Run("a tie among the two newest keeps the earlier of them", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		writeManifestFile(t, dir, "neru.yaml", manifestNamed("neru.yaml"), base.Add(-time.Hour))
		writeManifestFile(t, dir, "vcr.yaml", manifestNamed("vcr.yaml"), base)
		writeManifestFile(t, dir, vcrYml, manifestNamed(vcrYml), base)

		_, name, ok, err := p.ReadManifest()

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "vcr.yaml", name)
	})
}

func TestReadManifestNoneFound(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, "not a manifest", dir, "README.md")

	m, name, ok, err := p.ReadManifest()

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, m)
	assert.Empty(t, name)
}

func TestReadManifestParseFailure(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, "project: [\n", dir, vcrYml)

	m, name, ok, err := p.ReadManifest()

	require.Error(t, err)
	assert.Nil(t, m)
	assert.Equal(t, vcrYml, name)
	assert.True(t, ok)
}

func TestReadManifestFollowsALinkThatStaysInside(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, manifestNamed("linked"), dir, "real.yml")
	symlinkOrSkip(t, "real.yml", filepath.Join(dir, vcrYml))

	m, name, ok, err := p.ReadManifest()

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, vcrYml, name)
	assert.Equal(t, "linked", m.Project.Name)
}

func TestReadManifestRefusesAnEscapingLink(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	outside := t.TempDir()
	write(t, manifestNamed("outside"), outside, "secret.yml")
	write(t, manifestNamed("inside"), dir, "neru.yml")
	symlinkOrSkip(t, relativeLink(t, dir, filepath.Join(outside, "secret.yml")), filepath.Join(dir, vcrYml))

	m, name, ok, err := p.ReadManifest()

	require.Error(t, err)
	assert.Nil(t, m)
	assert.Empty(t, name)
	assert.False(t, ok)
}

func TestWriteManifestCreatesVcrYml(t *testing.T) {
	zeroUmask(t)
	p, dir := newProject(t)
	m := sampleManifest()

	require.NoError(t, p.WriteManifest(m, vcrYml))

	assert.Equal(t, marshalled(t, m), []byte(readString(t, dir, vcrYml)))
	assert.Equal(t, []string{vcrYml}, namesIn(t, dir))
	requireMode(t, 0o644, dir, vcrYml)
}

func TestWriteManifestRenamesTheTemplateManifest(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, "template content", dir, "neru.yml")
	write(t, "older vcr.yml", dir, vcrYml)
	m := sampleManifest()

	require.NoError(t, p.WriteManifest(m, "neru.yml"))

	requireMissing(t, dir, "neru.yml")
	assert.Equal(t, marshalled(t, m), []byte(readString(t, dir, vcrYml)))
	assert.Equal(t, []string{vcrYml}, namesIn(t, dir))
}

func TestWriteManifestKeepsTheNameWhenEqual(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, "an older and much longer manifest content that must not survive", dir, vcrYml)
	m := sampleManifest()

	require.NoError(t, p.WriteManifest(m, vcrYml))

	assert.Equal(t, marshalled(t, m), []byte(readString(t, dir, vcrYml)))
	assert.Equal(t, []string{vcrYml}, namesIn(t, dir))
}

func TestWriteManifestRefusesAPlantedLink(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			p, dir := newProject(t)
			outside := t.TempDir()
			victim := filepath.Join(outside, "victim")
			write(t, "original", victim)
			symlinkOrSkip(t, linkSpellings(t, dir, victim)[kind], filepath.Join(dir, vcrYml))

			err := p.WriteManifest(sampleManifest(), vcrYml)

			requirePrefix(t, err, fmt.Sprintf("failed to write manifest to %q: ", filepath.Join(dir, vcrYml)))
			assert.Equal(t, "original", readString(t, victim))
		})
	}
}

func TestWriteManifestReplacesALinkWhenRenaming(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	write(t, "original", victim)
	write(t, "template content", dir, "neru.yml")
	symlinkOrSkip(t, relativeLink(t, dir, victim), filepath.Join(dir, vcrYml))
	m := sampleManifest()

	require.NoError(t, p.WriteManifest(m, "neru.yml"))

	info, err := os.Lstat(filepath.Join(dir, vcrYml))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "vcr.yml is a regular file again")
	assert.Equal(t, marshalled(t, m), []byte(readString(t, dir, vcrYml)))
	assert.Equal(t, "original", readString(t, victim))
}

func TestWriteManifestRenameFailureText(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	p, dir := newProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, vcrYml, "inner"), 0o750))

	err := p.WriteManifest(sampleManifest(), "neru.yml")

	requirePrefix(t, err, fmt.Sprintf("failed to rename manifest file to %q : ", filepath.Join(dir, vcrYml)))
}
