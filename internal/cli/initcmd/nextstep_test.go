package initcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextStep(t *testing.T) {
	// The working directory is resolved first so that a temporary directory behind a link (macOS) cannot change
	// the relative path.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(base)
	unix := runtime.GOOS != "windows"

	tests := []struct {
		name     string
		dir      string
		wantPath string // the directory the hint names; empty when it is the working directory
		needUnix bool
	}{
		{name: "a directory inside the working directory is shown relative", dir: filepath.Join(base, "my-project"), wantPath: "my-project"},
		{
			name:     "a nested directory keeps its separators",
			dir:      filepath.Join(base, "a", "b"),
			wantPath: shellQuote(filepath.Join("a", "b")),
		},
		{name: "the working directory itself needs no cd", dir: base},
		{name: "a name with a space is quoted", dir: filepath.Join(base, "my project"), wantPath: "'my project'"},
		{
			name:     "a sibling of the working directory is shown whole",
			dir:      filepath.Join(filepath.Dir(base), "sibling"),
			wantPath: shellQuote(filepath.Join(filepath.Dir(base), "sibling")),
			needUnix: true,
		},
		{
			name:     "the parent of the working directory is shown whole",
			dir:      filepath.Dir(base),
			wantPath: shellQuote(filepath.Dir(base)),
			needUnix: true,
		},
		{name: "a single quote is escaped", dir: filepath.Join(base, "it's"), wantPath: `'it'\''s'`, needUnix: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needUnix && !unix {
				t.Skip("an absolute path holds backslashes here, and a name cannot hold a quote")
			}
			want := "Next: vcr deploy"
			if tt.wantPath != "" {
				want = "Next: cd " + tt.wantPath + " && vcr deploy"
			}

			assert.Equal(t, want, nextStep(tt.dir))
		})
	}
}

func TestNextStepNamesTheAbsolutePathWhenTheWorkingDirectoryIsGone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory in use cannot be removed here")
	}
	gone := filepath.Join(t.TempDir(), "gone")
	require.NoError(t, os.Mkdir(gone, 0o750))
	t.Chdir(gone)
	require.NoError(t, os.Remove(gone))
	if _, err := os.Getwd(); err == nil {
		t.Skip("this system still reports a working directory that was removed")
	}
	dir := filepath.Join(t.TempDir(), "my-project")

	assert.Equal(t, "Next: cd "+shellQuote(dir)+" && vcr deploy", nextStep(dir))
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"plain", "plain"},
		{"with space", "'with space'"},
		{"$HOME", "'$HOME'"},
		{"a'b", `'a'\''b'`},
		{"safe/path.d:1@x%y+z=w,v-u_t", "safe/path.d:1@x%y+z=w,v-u_t"},
		{"semi;colon", "'semi;colon'"},
		{"back`tick", "'back`tick'"},
		{"star*", "'star*'"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, shellQuote(tt.in))
		})
	}
}
