//go:build unix

package init

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

// zeroUmask clears the process umask until the test ends, so a created file or directory shows exactly the mode that
// was asked for; under the usual 022 the umask alone would hide a group or other write bit that was not dropped.
// The umask belongs to the whole process, so a test that calls this must not call t.Parallel.
func zeroUmask(t *testing.T) {
	t.Helper()
	previous := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(previous) })
}

func requireMode(t *testing.T, want os.FileMode, elem ...string) {
	t.Helper()
	path := filepath.Join(elem...)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), "mode of %s", path)
}

func TestUncompressToDirCreatesDirectoriesWithMode0755(t *testing.T) {
	zeroUmask(t)
	dir := newProjectDir(t)
	private := directory("private/")
	private.mode = 0o700

	require.NoError(t, uncompressToDir(tarball(t, private, file("nested/inner/x.txt", "x")), dir))

	requireMode(t, 0o755, dir, "private")
	requireMode(t, 0o755, dir, "nested")
	requireMode(t, 0o755, dir, "nested", "inner")
}

func TestUncompressToDirModeRule(t *testing.T) {
	zeroUmask(t)
	dir := newProjectDir(t)
	rows := []struct {
		name string
		mode int64
		want os.FileMode
	}{
		{"private", 0o600, 0o600},
		{"owner-executable", 0o700, 0o700},
		{"world-writable", 0o666, 0o644},
		{"setuid", 0o4777, 0o755},
		{"no-bits", 0, 0o644},
	}
	entries := make([]entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, entry{name: "modes/" + row.name, typeflag: tar.TypeReg, mode: row.mode, body: "x"})
	}

	require.NoError(t, uncompressToDir(tarball(t, entries...), dir))

	const special = os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			requireMode(t, row.want, dir, "modes", row.name)
			info, err := os.Stat(filepath.Join(dir, "modes", row.name))
			require.NoError(t, err)
			assert.Zero(t, info.Mode()&special, "no setuid, setgid or sticky bit is kept")
		})
	}
}

func TestInitCreatesAMissingProjectDirectoryWithMode0755(t *testing.T) {
	zeroUmask(t)
	dir := filepath.Join(t.TempDir(), "new-project")
	ctrl := gomock.NewController(t)
	surveyMock := mocks.NewMockSurveyInterface(ctrl)
	surveyMock.EXPECT().AskForUserInput(gomock.Any(), gomock.Any()).Return("", errors.New("stop here"))
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdInit(testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, surveyMock, nil))
	cmd.SetArgs([]string{dir})
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()

	require.ErrorContains(t, err, "failed to ask project name")
	requireMode(t, 0o755, dir)
}
