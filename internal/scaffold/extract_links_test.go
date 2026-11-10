package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/scaffold"
)

// linkSpellings returns the two ways a link at linkDir can name target: the absolute path and the relative one.
func linkSpellings(t *testing.T, linkDir, target string) map[string]string {
	t.Helper()
	return map[string]string{
		"absolute": target,
		"relative": relativeLink(t, linkDir, target),
	}
}

func TestExtractFailsThroughASymlinkLeadingOutside(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			p, dir := newProject(t)
			outside := t.TempDir()
			symlinkOrSkip(t, linkSpellings(t, dir, outside)[kind], filepath.Join(dir, "static"))

			err := p.Extract(tarball(t, file("static/x", "planted")), scaffold.DefaultLimits)

			require.Error(t, err)
			assert.Empty(t, namesIn(t, outside))
		})
	}
}

func TestExtractFailsThroughALinkedDirectoryEntry(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	outside := t.TempDir()
	symlinkOrSkip(t, relativeLink(t, dir, outside), filepath.Join(dir, "static"))

	err := p.Extract(tarball(t, directory("static/sub/")), scaffold.DefaultLimits)

	require.Error(t, err)
	assert.Empty(t, namesIn(t, outside))
}

func TestExtractDoesNotWriteThroughAPlantedFileLink(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			p, dir := newProject(t)
			outside := t.TempDir()
			victim := filepath.Join(outside, "victim")
			write(t, "original", victim)
			symlinkOrSkip(t, linkSpellings(t, dir, victim)[kind], filepath.Join(dir, "vcr.yml"))

			err := p.Extract(tarball(t, file("vcr.yml", "project: planted\n")), scaffold.DefaultLimits)

			require.Error(t, err)
			assert.Equal(t, "original", readString(t, victim))
		})
	}
}

func TestExtractAllowsASymlinkThatStaysInside(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "real"), 0o750))
	symlinkOrSkip(t, "real", filepath.Join(dir, "inside"))

	require.NoError(t, p.Extract(tarball(t, file("inside/x", "through the link")), scaffold.DefaultLimits))

	assert.Equal(t, "through the link", readString(t, dir, "real", "x"))
}
