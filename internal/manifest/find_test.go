package manifest_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

const (
	errNoFlagManifest     = "failed to find flag manifest file: manifest file not found"
	errNoTemplateManifest = "failed to find template manifest file: manifest file not found"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// layout creates a project directory holding one empty file per entry, each with its modification time.
func layout(t *testing.T, modTimes map[string]time.Time) string {
	t.Helper()
	dir := t.TempDir()
	for name, when := range modTimes {
		setModTime(t, writeFile(t, dir, name, ""), when)
	}
	return dir
}

func TestFindFlagPathUsedAsIs(t *testing.T) {
	root := t.TempDir()
	elsewhere := filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(elsewhere, 0o750))
	file := writeFile(t, elsewhere, "custom.yml", "")
	projectDir := filepath.Join(root, "project")
	require.NoError(t, os.Mkdir(projectDir, 0o750))
	t.Chdir(root)

	tests := []struct {
		name     string
		flagPath string
	}{
		{name: "an absolute file", flagPath: file},
		{name: "a relative file stays relative to the working directory", flagPath: filepath.Join("elsewhere", "custom.yml")},
		{name: "a directory is accepted without looking inside", flagPath: elsewhere},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.Find(projectDir, tt.flagPath)
			require.NoError(t, err)
			assert.Equal(t, tt.flagPath, got)
		})
	}
}

func TestFindFlagPathMissing(t *testing.T) {
	dir := layout(t, map[string]time.Time{"vcr.yml": epoch})

	got, err := manifest.Find(dir, filepath.Join(dir, "nope.yml"))

	assert.EqualError(t, err, errNoFlagManifest)
	assert.Empty(t, got)
}

func TestFindStatErrorsAreReportedWithTheirOwnText(t *testing.T) {
	skipOnWindows(t)
	file := writeFile(t, t.TempDir(), "plain.txt", "")

	t.Run("flag path", func(t *testing.T) {
		got, err := manifest.Find(t.TempDir(), filepath.Join(file, "vcr.yml"))

		require.ErrorIs(t, err, syscall.ENOTDIR)
		assert.ErrorContains(t, err, "failed to find flag manifest file: stat "+file+"/vcr.yml: not a directory")
		assert.Empty(t, got)
	})
	t.Run("project directory", func(t *testing.T) {
		got, err := manifest.Find(file, "")

		require.ErrorIs(t, err, syscall.ENOTDIR)
		assert.EqualError(t, err, "failed to find template manifest file: stat "+file+"/neru.yaml: not a directory")
		assert.Empty(t, got)
	})
}

func TestFindNewestModificationTimeWins(t *testing.T) {
	for _, newest := range manifest.Names {
		t.Run(newest, func(t *testing.T) {
			modTimes := make(map[string]time.Time, len(manifest.Names))
			for i, name := range manifest.Names {
				modTimes[name] = epoch.Add(-time.Duration(i+1) * time.Hour)
			}
			modTimes[newest] = epoch
			dir := layout(t, modTimes)

			got, err := manifest.Find(dir, "")

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, newest), got)
		})
	}
}

func TestFindTieGoesToTheEarlierName(t *testing.T) {
	older := epoch.Add(-time.Hour)
	tests := []struct {
		name     string
		modTimes map[string]time.Time
		want     string
	}{
		{
			name: "all four equal",
			modTimes: map[string]time.Time{
				"neru.yaml": epoch, "neru.yml": epoch, "vcr.yaml": epoch, "vcr.yml": epoch,
			},
			want: "neru.yaml",
		},
		{
			name: "the two vcr names are equal and newest",
			modTimes: map[string]time.Time{
				"neru.yaml": older, "neru.yml": older, "vcr.yaml": epoch, "vcr.yml": epoch,
			},
			want: "vcr.yaml",
		},
		{
			name: "the later names of each family are equal and newest",
			modTimes: map[string]time.Time{
				"neru.yaml": older, "neru.yml": epoch, "vcr.yaml": older, "vcr.yml": epoch,
			},
			want: "neru.yml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := layout(t, tt.modTimes)

			got, err := manifest.Find(dir, "")

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, tt.want), got)
		})
	}
}

func TestFindSingleManifestWins(t *testing.T) {
	for _, name := range manifest.Names {
		t.Run(name, func(t *testing.T) {
			dir := layout(t, map[string]time.Time{name: epoch})

			got, err := manifest.Find(dir, "")

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, name), got)
		})
	}
}

func TestFindNoManifest(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "an empty directory", dir: t.TempDir()},
		{name: "a directory with another name", dir: layout(t, map[string]time.Time{"manifest.yml": epoch})},
		{name: "a directory that does not exist", dir: filepath.Join(t.TempDir(), "missing")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.Find(tt.dir, "")

			assert.EqualError(t, err, errNoTemplateManifest)
			assert.Empty(t, got)
		})
	}
}

func TestFindFollowsSymbolicLinks(t *testing.T) {
	setup := func(t *testing.T, targetTime time.Time) string {
		t.Helper()
		target := writeFile(t, t.TempDir(), "real.yml", "")
		setModTime(t, target, targetTime)
		dir := layout(t, map[string]time.Time{"vcr.yml": epoch})
		symlinkOrSkip(t, target, filepath.Join(dir, "neru.yml"))
		return dir
	}

	t.Run("a link to a newer file counts as newer", func(t *testing.T) {
		dir := setup(t, epoch.Add(time.Hour))

		got, err := manifest.Find(dir, "")

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "neru.yml"), got)
	})
	t.Run("a link to an older file counts as older, however new the link is", func(t *testing.T) {
		dir := setup(t, epoch.Add(-time.Hour))

		got, err := manifest.Find(dir, "")

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "vcr.yml"), got)
	})
}

func TestFindCountsADirectoryNamedLikeAManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "vcr.yml"), 0o750))

	got, err := manifest.Find(dir, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "vcr.yml"), got)

	_, err = manifest.Read(got)
	assert.Error(t, err)
}
