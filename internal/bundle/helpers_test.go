package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const readLimit = 1 << 20

// writeTree creates the files of tree below root, with their parent directories. A name that ends in a slash makes an
// empty directory. Files get mode 0644, whatever the umask says.
func writeTree(t *testing.T, root string, tree map[string]string) {
	t.Helper()
	for name, content := range tree {
		target := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			require.NoError(t, os.MkdirAll(target, 0o750))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
		require.NoError(t, os.WriteFile(target, []byte(content), 0o600))
		require.NoError(t, os.Chmod(target, 0o644))
	}
}

// symlinkOrSkip makes a link at path, or skips the test where the system does not allow links.
func symlinkOrSkip(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("links are not available here: %v", err)
	}
}

// skipUnlessPermissionsApply skips tests that rely on mode bits keeping the current user out.
func skipUnlessPermissionsApply(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("mode bits do not keep this user out")
	}
}

// mustList lists root and fails the test on an error. It returns the paths and what was written to warn.
func mustList(t *testing.T, root string) (paths []string, warn string) {
	t.Helper()
	var out bytes.Buffer
	entries, err := List(context.Background(), root, &out)
	require.NoError(t, err)
	return pathsOf(entries), out.String()
}

func pathsOf(entries []Entry) []string {
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	return paths
}

// archived is one entry read back from an archive.
type archived struct {
	header tar.Header
	body   string
}

// readArchive decompresses the tar.gz at path and returns its entries in archive order.
func readArchive(t *testing.T, path string) []archived {
	t.Helper()
	zr := openGzip(t, path)
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(zr)
	var entries []archived
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return entries
		}
		require.NoError(t, err)
		body, err := io.ReadAll(io.LimitReader(tr, readLimit))
		require.NoError(t, err)
		entries = append(entries, archived{header: *hdr, body: string(body)})
	}
}

// readTarBytes returns the decompressed bytes of the tar.gz at path.
func readTarBytes(t *testing.T, path string) []byte {
	t.Helper()
	zr := openGzip(t, path)
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, readLimit))
	require.NoError(t, err)
	return data
}

func openGzip(t *testing.T, path string) *gzip.Reader {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	return zr
}

// namesOf returns the names of the archive entries.
func namesOf(entries []archived) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.header.Name)
	}
	return names
}

// permOf returns the permission bits lstat reports for path, as the archive header carries them.
func permOf(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	return int64(info.Mode().Perm())
}

// useTempDir points the temporary directory at dir for the rest of the test, so the test sees every file Build leaves
// behind. Create every other directory the test needs before calling it.
func useTempDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Empty(t, names, "files left behind in %s", dir)
}
