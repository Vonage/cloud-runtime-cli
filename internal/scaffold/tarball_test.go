package scaffold_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// entry is one member of a test archive. Only the header fields the extractor looks at are here, and mode is written
// as given, so a zero mode really is a zero mode.
type entry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
}

func file(name, body string) entry {
	return entry{name: name, typeflag: tar.TypeReg, mode: 0o644, body: body}
}

func directory(name string) entry {
	return entry{name: name, typeflag: tar.TypeDir, mode: 0o755}
}

// tarBytes returns the plain tar archive of entries.
func tarBytes(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		header := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     e.mode,
			Linkname: e.linkname,
			Size:     int64(len(e.body)),
		}
		require.NoError(t, tw.WriteHeader(header))
		_, err := io.WriteString(tw, e.body)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(data)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// tarball returns the tar.gz archive of entries, the format a template is downloaded in.
func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	return gzipped(t, tarBytes(t, entries...))
}

// zeroReader is an endless source of zero bytes.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// bombArchive streams one regular file of size zero bytes through a fast gzip writer, so the archive stays small
// while the file it holds is as large as asked.
func bombArchive(t *testing.T, size int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	require.NoError(t, err)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "bomb.bin", Typeflag: tar.TypeReg, Mode: 0o644, Size: size}))
	_, err = io.CopyN(tw, zeroReader{}, size)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}
