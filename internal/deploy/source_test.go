package deploy

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const failedToCompress = "failed to compress and upload file: failed to compress directory "

// writeTgz writes a gzip-compressed tar archive with one file called name and returns its path.
func writeTgz(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, "app.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := gzip.NewWriter(f)
	tw := tar.NewWriter(zw)
	body := []byte("console.log('packed');\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}))
	_, err = tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return path
}

func skipUnlessPermissionsApply(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("mode bits do not keep this user out")
	}
}

func TestRunTgzIsUploadedAsIs(t *testing.T) {
	h := newHarness(t)
	tmp := tempDirHolder(t)
	h.plan.TgzPath = writeTgz(t, t.TempDir(), "packed.js")

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, h.plan.TgzPath, h.api.uploadedPath, "the file is uploaded where it is, not copied")
	assert.Equal(t, []string{"packed.js"}, h.api.entries, "the project directory is not packaged")
	assert.Contains(t, h.stdout.String(), "✓ Source code uploaded.\n")
	requireEmptyDir(t, tmp)
	_, statErr := os.Stat(h.plan.TgzPath)
	assert.NoError(t, statErr, "the archive is the user's and stays")
}

func TestRunTgzPathIsUsedAsTyped(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeTgz(t, dir, "packed.js")
	t.Chdir(dir)
	h.plan.TgzPath = "app.tar.gz"

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "app.tar.gz", h.api.uploadedPath)
}

func TestRunTgzErrors(t *testing.T) {
	notGzip := func(t *testing.T) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "plain.tgz")
		require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))
		return path
	}
	missing := func(t *testing.T) string {
		t.Helper()
		return filepath.Join(t.TempDir(), "gone.tgz")
	}
	valid := func(t *testing.T) string {
		t.Helper()
		return writeTgz(t, t.TempDir(), "packed.js")
	}

	tests := []struct {
		name      string
		path      func(*testing.T) string
		uploadErr error
		want      func(path string) string
		wantCalls []string
	}{
		{
			name: "a file that is not gzip",
			path: notGzip,
			want: func(path string) string { return fmt.Sprintf("%q is not a valid compressed file", path) },
		},
		{
			name: "a file that is not there",
			path: missing,
			want: func(path string) string {
				_, err := os.Open(path)
				return fmt.Sprintf("unable to read compressed file %q: %v", path, err)
			},
		},
		{
			name:      "an upload that fails",
			path:      valid,
			uploadErr: errors.New("boom"),
			want:      func(path string) string { return fmt.Sprintf("failed to upload compressed file %q: boom", path) },
			wantCalls: []string{callGetProject, callValidate, callUploadSource},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.plan.TgzPath = tt.path(t)
			h.api.uploadErr = tt.uploadErr

			err := h.run()

			require.EqualError(t, err, "failed to read and upload compressed file : "+tt.want(h.plan.TgzPath))
			if tt.wantCalls == nil {
				tt.wantCalls = []string{callGetProject, callValidate}
			}
			assert.Equal(t, tt.wantCalls, h.api.calls)
			assert.NotContains(t, h.stdout.String(), "Source code uploaded")
		})
	}
}

func TestRunUploadFailure(t *testing.T) {
	h := newHarness(t)
	h.api.uploadErr = errors.New("boom")
	tmp := tempDirHolder(t)

	err := h.run()

	require.EqualError(t, err, "failed to compress and upload file: failed to upload compressed file: boom")
	assert.False(t, h.api.called(callCreatePackage))
	requireEmptyDir(t, tmp)
}

func TestRunEmptyProject(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, dir, ".vcrignore", "ignored.txt\n")
	writeFile(t, dir, "ignored.txt", "left out\n")
	h.plan.ProjectDir = dir
	tmp := tempDirHolder(t)

	err := h.run()

	require.EqualError(t, err, "failed to compress and upload file: directory "+dir+" does not contain any source code")
	assert.False(t, h.api.called(callUploadSource))
	requireEmptyDir(t, tmp)
}

func TestRunWarningsReachStderrAfterCompress(t *testing.T) {
	skipUnlessPermissionsApply(t)
	h := newHarness(t)
	writeFile(t, h.plan.ProjectDir, "locked.txt", "secret\n")
	require.NoError(t, os.Chmod(filepath.Join(h.plan.ProjectDir, "locked.txt"), 0))

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "! Skipping file locked.txt due to error: open locked.txt: permission denied\n", h.stderr.String())
	assert.Equal(t, []string{"index.js"}, h.api.entries)
	assert.NotContains(t, h.stdout.String(), "Skipping")
}

func TestRunWarningsAreSanitised(t *testing.T) {
	skipUnlessPermissionsApply(t)
	h := newHarness(t)
	name := "a\x1b[2Jb.txt"
	path := filepath.Join(h.plan.ProjectDir, name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Skipf("this file system does not take control characters in names: %v", err)
	}
	require.NoError(t, os.Chmod(path, 0))

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "! Skipping file ab.txt due to error: open ab.txt: permission denied\n", h.stderr.String())
}

func TestRunWarningsAreDroppedWhenCompressFails(t *testing.T) {
	skipUnlessPermissionsApply(t)
	h := newHarness(t)
	writeFile(t, h.plan.ProjectDir, "a-locked.txt", "secret\n")
	require.NoError(t, os.Chmod(filepath.Join(h.plan.ProjectDir, "a-locked.txt"), 0))
	blocked := filepath.Join(h.plan.ProjectDir, "z-blocked")
	require.NoError(t, os.Mkdir(blocked, 0o750))
	require.NoError(t, os.Chmod(blocked, 0))
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o750) })
	tmp := tempDirHolder(t)

	err := h.run()

	require.Error(t, err)
	assert.Contains(t, err.Error(), failedToCompress+fmt.Sprintf("%q", h.plan.ProjectDir))
	assert.Contains(t, err.Error(), "permission denied")
	assert.Empty(t, h.stderr.String(), "v2.6.1 printed nothing for a compress that failed")
	assert.False(t, h.api.called(callUploadSource))
	requireEmptyDir(t, tmp)
}

func TestRunDefaultIgnoreNotice(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, dir, ".git/config", "[core]\n")
	writeFile(t, dir, ".env", "A=1\n")
	writeFile(t, dir, "app.js", "console.log(1);\n")
	h.plan.ProjectDir = dir

	err := h.run()

	require.NoError(t, err)
	want := `! Skipped 1 file(s) under .git/ and 1 .env file(s) so history and secrets are not uploaded. ` +
		`To include them, add "!.git/" or "!.env" to .vcrignore.` + "\n"
	assert.Equal(t, want, h.stderr.String())
	assert.Equal(t, []string{"app.js"}, h.api.entries)
}

func TestRunUploadsEveryFileOfTheProject(t *testing.T) {
	h := newHarness(t)
	writeFile(t, h.plan.ProjectDir, "src/lib/util.js", "module.exports = 1;\n")
	writeFile(t, h.plan.ProjectDir, ".jfs.config", "skipped\n")

	err := h.run()

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"index.js", "src/lib/util.js"}, h.api.entries)
	assert.Empty(t, h.stderr.String())
}
