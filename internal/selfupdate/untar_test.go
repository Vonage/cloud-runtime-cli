package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reasonBadArchive = "the archive does not contain exactly one file"

// tarEntry is one entry of a test archive.
type tarEntry struct {
	header  tar.Header
	content []byte
}

func regularFile(name string, content []byte) tarEntry {
	return tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o755}, content: content}
}

func specialEntry(typeflag byte, name, link string) tarEntry {
	return tarEntry{header: tar.Header{Typeflag: typeflag, Name: name, Linkname: link, Mode: 0o755}}
}

// tarBytes is the uncompressed tar of entries.
func tarBytes(tb testing.TB, entries ...tarEntry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := e.header
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.content))
		}
		require.NoError(tb, tw.WriteHeader(&h))
		_, err := tw.Write(e.content)
		require.NoError(tb, err)
	}
	require.NoError(tb, tw.Close())
	return buf.Bytes()
}

func gzipBytes(tb testing.TB, data []byte) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(data)
	require.NoError(tb, err)
	require.NoError(tb, zw.Close())
	return buf.Bytes()
}

// archiveOf is a gzip-compressed tar of entries.
func archiveOf(tb testing.TB, entries ...tarEntry) []byte {
	tb.Helper()
	return gzipBytes(tb, tarBytes(tb, entries...))
}

// countingWriter keeps what it was given and fails after failAfter bytes when that is positive.
type countingWriter struct {
	buf       bytes.Buffer
	failAfter int
}

var errDiskFull = errors.New("disk full")

func (w *countingWriter) Write(p []byte) (int, error) {
	if w.failAfter > 0 && w.buf.Len()+len(p) > w.failAfter {
		return 0, errDiskFull
	}
	return w.buf.Write(p)
}

func TestExtractSingleAcceptsExactlyOneRegularFile(t *testing.T) {
	body := []byte("#!/bin/sh\necho vcr\n")
	names := []string{
		"vcr", "./vcr", "vcr_linux_amd64", "dist/vcr", "../../etc/passwd", "/usr/local/bin/vcr", strings.Repeat("a", 300),
	}
	for _, name := range names {
		t.Run("named "+name[:min(len(name), 20)], func(t *testing.T) {
			var dst countingWriter

			err := extractSingle(archiveOf(t, regularFile(name, body)), &dst, 1<<20)

			require.NoError(t, err)
			assert.Equal(t, body, dst.buf.Bytes())
		})
	}

	t.Run("a file exactly at the limit", func(t *testing.T) {
		var dst countingWriter

		err := extractSingle(archiveOf(t, regularFile("vcr", body)), &dst, int64(len(body)))

		require.NoError(t, err)
		assert.Equal(t, body, dst.buf.Bytes())
	})
	t.Run("a file that is empty", func(t *testing.T) {
		var dst countingWriter

		require.NoError(t, extractSingle(archiveOf(t, regularFile("vcr", nil)), &dst, 10))
		assert.Zero(t, dst.buf.Len())
	})
}

func TestExtractSingleRefusesEverythingElse(t *testing.T) {
	body := []byte("#!/bin/sh\necho vcr\n")
	valid := archiveOf(t, regularFile("vcr", body))
	emptyTar := tarBytes(t)

	tests := []struct {
		name    string
		archive []byte
	}{
		{"two files", archiveOf(t, regularFile("vcr", body), regularFile("other", body))},
		{"the same name twice", archiveOf(t, regularFile("vcr", body), regularFile("vcr", body))},
		{"a file and a directory", archiveOf(t, regularFile("vcr", body), specialEntry(tar.TypeDir, "dir/", ""))},
		{"a directory and a file", archiveOf(t, specialEntry(tar.TypeDir, "dir/", ""), regularFile("dir/vcr", body))},
		{"only a directory", archiveOf(t, specialEntry(tar.TypeDir, "dir/", ""))},
		{"a symlink", archiveOf(t, specialEntry(tar.TypeSymlink, "vcr", "/bin/sh"))},
		{"a symlink after the file", archiveOf(t, regularFile("vcr", body), specialEntry(tar.TypeSymlink, "link", "vcr"))},
		{"a hard link", archiveOf(t, specialEntry(tar.TypeLink, "vcr", "other"))},
		{"a character device", archiveOf(t, specialEntry(tar.TypeChar, "vcr", ""))},
		{"a block device", archiveOf(t, specialEntry(tar.TypeBlock, "vcr", ""))},
		{"a named pipe", archiveOf(t, specialEntry(tar.TypeFifo, "vcr", ""))},
		{"an empty tar", gzipBytes(t, emptyTar)},
		{"an empty gzip stream", gzipBytes(t, nil)},
		{"no bytes at all", nil},
		{"not gzip", []byte("this is not an archive at all")},
		{"a tar that was not compressed", tarBytes(t, regularFile("vcr", body))},
		{"gzip that stops early", valid[:len(valid)/2]},
		{"gzip without its trailer", valid[:len(valid)-8]},
		{"gzip with a damaged checksum", flipBit(valid, len(valid)-8)},
		{"a tar header that is damaged", gzipBytes(t, flipBit(tarBytes(t, regularFile("vcr", body)), 100))},
		{"a global header before the file", archiveOf(t, globalHeader(), regularFile("vcr", body))},
		{"junk after the file", gzipBytes(t, tarWithJunk(t, regularFile("vcr", body)))},
		{"a tar that stops inside the file", gzipBytes(t, tarBytes(t, regularFile("vcr", body))[:512+4])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst countingWriter

			err := extractSingle(tt.archive, &dst, 1<<20)

			require.Error(t, err)
			var shape *archiveError
			require.ErrorAs(t, err, &shape)
			assert.Equal(t, reasonBadArchive, err.Error())
		})
	}
}

// flipBit returns a copy of b with one bit of byte i flipped.
func flipBit(b []byte, i int) []byte {
	out := bytes.Clone(b)
	out[i] ^= 0x01
	return out
}

func globalHeader() tarEntry {
	return tarEntry{header: tar.Header{
		Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "made by a tool"},
	}}
}

// tarWithJunk is a tar whose only entry is followed by a block that is not a header.
func tarWithJunk(tb testing.TB, e tarEntry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	h := e.header
	h.Size = int64(len(e.content))
	require.NoError(tb, tw.WriteHeader(&h))
	_, err := tw.Write(e.content)
	require.NoError(tb, err)
	require.NoError(tb, tw.Flush())
	buf.Write(bytes.Repeat([]byte("x"), 512))
	require.NoError(tb, tw.Close())
	return buf.Bytes()
}

func TestExtractSingleStopsAtTheLimit(t *testing.T) {
	t.Run("a file over the limit is refused before anything is written", func(t *testing.T) {
		var dst countingWriter

		err := extractSingle(archiveOf(t, regularFile("vcr", bytes.Repeat([]byte("x"), 11))), &dst, 10)

		var shape *archiveError
		require.ErrorAs(t, err, &shape)
		assert.Zero(t, dst.buf.Len())
	})
	t.Run("a large file of zeros stops at the limit", func(t *testing.T) {
		const limit = 1 << 10
		var dst countingWriter

		err := extractSingle(archiveOf(t, regularFile("vcr", make([]byte, 8<<20))), &dst, limit)

		var shape *archiveError
		require.ErrorAs(t, err, &shape)
		assert.LessOrEqual(t, dst.buf.Len(), limit+1)
	})
}

func TestExtractSingleReportsAWriteFailureAsItself(t *testing.T) {
	dst := &countingWriter{failAfter: 4}

	err := extractSingle(archiveOf(t, regularFile("vcr", []byte("0123456789"))), dst, 1<<20)

	require.ErrorIs(t, err, errDiskFull)
	var shape *archiveError
	assert.False(t, errors.As(err, &shape), "a full disk is not a bad archive")
}
