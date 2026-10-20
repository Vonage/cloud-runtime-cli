package upgrade

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSingleReturnsExactlyOneRegularFile(t *testing.T) {
	body := []byte("#!/bin/sh\necho vcr\n")
	names := []string{"vcr", "./vcr", "vcr_linux_amd64", "dist/vcr", "../../etc/passwd", "/usr/local/bin/vcr", strings.Repeat("a", 300)}
	for _, name := range names {
		t.Run("named "+name[:min(len(name), 20)], func(t *testing.T) {
			got, err := extractSingle(archiveOf(t, regularFile(name, body)), 1<<20)

			require.NoError(t, err)
			assert.Equal(t, body, got)
		})
	}

	t.Run("a file exactly at the limit", func(t *testing.T) {
		got, err := extractSingle(archiveOf(t, regularFile("vcr", body)), int64(len(body)))

		require.NoError(t, err)
		assert.Equal(t, body, got)
	})
}

func TestExtractSingleRefusesEverythingElse(t *testing.T) {
	body := []byte("#!/bin/sh\necho vcr\n")
	valid := archiveOf(t, regularFile("vcr", body))

	tests := []struct {
		name    string
		archive []byte
	}{
		{"two files", archiveOf(t, regularFile("vcr", body), regularFile("other", body))},
		{"a file and a directory", archiveOf(t, regularFile("vcr", body), specialEntry(tar.TypeDir, "dir/", ""))},
		{"only a directory", archiveOf(t, specialEntry(tar.TypeDir, "dir/", ""))},
		{"a symlink", archiveOf(t, specialEntry(tar.TypeSymlink, "vcr", "/bin/sh"))},
		{"a symlink after the file", archiveOf(t, regularFile("vcr", body), specialEntry(tar.TypeSymlink, "link", "vcr"))},
		{"a hard link", archiveOf(t, specialEntry(tar.TypeLink, "vcr", "other"))},
		{"a named pipe", archiveOf(t, specialEntry(tar.TypeFifo, "vcr", ""))},
		{"an empty tar", gzipBytes(t, tarBytes(t))},
		{"a file with no bytes", archiveOf(t, regularFile("vcr", nil))},
		{"no bytes at all", nil},
		{"not gzip", []byte("this is not an archive at all")},
		{"a tar that was not compressed", tarBytes(t, regularFile("vcr", body))},
		{"gzip that stops early", valid[:len(valid)/2]},
		{"gzip without its trailer", valid[:len(valid)-8]},
		{"gzip with a damaged checksum", flipBit(valid, len(valid)-8)},
		{"a tar that stops inside the file", gzipBytes(t, tarBytes(t, regularFile("vcr", body))[:512+4])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractSingle(tt.archive, 1<<20)

			var shape *archiveError
			require.ErrorAs(t, err, &shape)
			assert.Equal(t, reasonBadArchive, err.Error())
			assert.Nil(t, got)
		})
	}
}

func TestExtractSingleStopsAtTheLimit(t *testing.T) {
	t.Run("a file over the limit", func(t *testing.T) {
		_, err := extractSingle(archiveOf(t, regularFile("vcr", bytes.Repeat([]byte("x"), 11))), 10)

		var shape *archiveError
		require.ErrorAs(t, err, &shape)
	})
	t.Run("a large file of zeros", func(t *testing.T) {
		_, err := extractSingle(archiveOf(t, regularFile("vcr", make([]byte, 8<<20))), 1<<10)

		var shape *archiveError
		require.ErrorAs(t, err, &shape)
	})
}
