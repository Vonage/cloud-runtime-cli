package bundle

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFileBytes(t *testing.T, path string, data []byte) string {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestCheckTgz(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stream bytes.Buffer
	zw := gzip.NewWriter(&stream)
	_, err := zw.Write([]byte("not really a tar, but a real gzip stream"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	valid := []struct {
		name string
		path string
	}{
		{"real gzip", writeFileBytes(t, filepath.Join(dir, "real.tgz"), stream.Bytes())},
		{"gzip magic followed by garbage", writeFileBytes(t, filepath.Join(dir, "garbage.tgz"), []byte{0x1f, 0x8b, 'x', 'y'})},
		{"gzip magic alone", writeFileBytes(t, filepath.Join(dir, "magic.tgz"), []byte{0x1f, 0x8b})},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, CheckTgz(tc.path))
		})
	}

	invalid := []struct {
		name string
		path string
	}{
		{"empty file", writeFileBytes(t, filepath.Join(dir, "empty.tgz"), nil)},
		{"one byte", writeFileBytes(t, filepath.Join(dir, "one.tgz"), []byte{0x1f})},
		{"plain text", writeFileBytes(t, filepath.Join(dir, "text.tgz"), []byte("hello world"))},
		{"zip magic", writeFileBytes(t, filepath.Join(dir, "zip.tgz"), []byte("PK\x03\x04rest"))},
		{"swapped magic", writeFileBytes(t, filepath.Join(dir, "swapped.tgz"), []byte{0x8b, 0x1f, 0})},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.EqualError(t, CheckTgz(tc.path), fmt.Sprintf("%q is not a valid compressed file", tc.path))
		})
	}
}

func TestCheckTgzMissingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.tgz")

	err := CheckTgz(path)

	require.ErrorIs(t, err, os.ErrNotExist)
	if runtime.GOOS != "windows" {
		assert.EqualError(t, err, fmt.Sprintf("unable to read compressed file %q: open %s: no such file or directory", path, path))
	}
}

func TestCheckTgzDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reading a directory fails differently on this system")
	}
	t.Parallel()
	path := t.TempDir()

	err := CheckTgz(path)

	assert.EqualError(t, err, fmt.Sprintf("unable to read compressed file %q: read %s: is a directory", path, path))
}
