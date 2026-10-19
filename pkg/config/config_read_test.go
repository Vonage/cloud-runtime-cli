package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCLIConfigTightensLooseFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	tests := []struct {
		mode os.FileMode
		want os.FileMode
	}{
		{mode: 0o644, want: 0o600},
		{mode: 0o640, want: 0o600},
		{mode: 0o666, want: 0o600},
		{mode: 0o604, want: 0o600},
		{mode: 0o600, want: 0o600},
		{mode: 0o400, want: 0o400},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "conf")
			writeFile(t, path, writtenConfig, tc.mode)

			got, err := ReadCLIConfig(path)

			require.NoError(t, err)
			assert.Equal(t, writtenValues, got)
			assert.Equal(t, tc.want, permOf(t, path))
		})
	}
}

func TestReadCLIConfigTightensSymlinkTarget(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	writeFile(t, target, writtenConfig, 0o644)
	symlinkOrSkip(t, target, link)

	_, err := ReadCLIConfig(link)

	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), permOf(t, target))
	requireSymlink(t, link)
}

func TestReadCLIConfigLeavesOtherUsersFileAlone(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, writtenConfig, 0o644)
	o := defaultOps
	o.uid = func() int { return os.Getuid() + 1 }

	got, err := readCLIConfig(o, path)

	require.NoError(t, err)
	assert.Equal(t, writtenValues, got)
	assert.Equal(t, os.FileMode(0o644), permOf(t, path))
}

func TestReadCLIConfigIgnoresChmodFailure(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, writtenConfig, 0o644)
	var chmodCalls int
	o := defaultOps
	o.chmod = func(string, os.FileMode) error {
		chmodCalls++
		return errors.New("chmod refused")
	}

	got, err := readCLIConfig(o, path)

	require.NoError(t, err)
	assert.Equal(t, writtenValues, got)
	assert.Equal(t, 1, chmodCalls, "the tightening was attempted")
}

func TestReadCLIConfigDoesNotChmodPrivateFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, writtenConfig, 0o600)
	o := defaultOps
	o.chmod = func(string, os.FileMode) error {
		t.Error("chmod must not be called for a private file")
		return nil
	}

	_, err := readCLIConfig(o, path)

	require.NoError(t, err)
}

func TestReadCLIConfigMissingFileIsNoConfig(t *testing.T) {
	t.Parallel()

	_, err := ReadCLIConfig(filepath.Join(t.TempDir(), "absent"))

	assert.ErrorIs(t, err, ErrNoConfig)
}
