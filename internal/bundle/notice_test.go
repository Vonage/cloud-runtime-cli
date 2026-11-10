package bundle

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	noticeGitAndEnv = `Skipped 3 file(s) under .git/ and 4 .env file(s) so history and secrets are not uploaded. ` +
		`To include them, add "!.git/" or "!.env" to .vcrignore.` + "\n"
	noticeFewerEnv = `Skipped 3 file(s) under .git/ and 2 .env file(s) so history and secrets are not uploaded. ` +
		`To include them, add "!.git/" or "!.env" to .vcrignore.` + "\n"
	noticeEnvOnly = `Skipped 4 .env file(s) so history and secrets are not uploaded. ` +
		`To include them, add "!.env" to .vcrignore.` + "\n"
	noticeGitOnly = `Skipped 3 file(s) under .git/ so history and secrets are not uploaded. ` +
		`To include them, add "!.git/" to .vcrignore.` + "\n"
	noticeOneEnv = `Skipped 1 .env file(s) so history and secrets are not uploaded. ` +
		`To include them, add "!.env" to .vcrignore.` + "\n"
)

// secretsTree holds history, local environment files, the mode files that stay, and look-alikes that stay.
func secretsTree() map[string]string {
	return map[string]string{
		".git/config":             "[core]",
		".git/objects/ab/cd":      "object",
		"sub/.git/x":              "x",
		".env":                    "A=1",
		"pkg/api/.env":            "A=2",
		".env.local":              "A=3",
		"a/.env.production.local": "A=4",
		".env.production":         "MODE=production",
		".env.example":            "A=",
		".envrc":                  "use nix",
		"app.js":                  "js",
		"worktree/.git":           "gitdir: /elsewhere",
	}
}

func withVcrignore(tree map[string]string, lines string) map[string]string {
	out := maps.Clone(tree)
	out[".vcrignore"] = lines
	return out
}

func TestDefaultIgnoresLeaveOutHistoryAndLocalSecrets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, secretsTree())

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{".env.example", ".env.production", ".envrc", "app.js", "worktree/.git"}, paths)
	assert.Equal(t, noticeGitAndEnv, warn)
}

func TestBangLinesBringDefaultsBack(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		lines  string
		paths  []string
		notice string
	}{
		{
			name:  "env",
			lines: "!.env\n",
			paths: []string{
				".env", ".env.example", ".env.production", ".envrc", "app.js", "pkg/api/.env", "worktree/.git",
			},
			notice: noticeFewerEnv,
		},
		{
			name:  "git",
			lines: "!.git/\n",
			paths: []string{
				".env.example", ".env.production", ".envrc", ".git/config", ".git/objects/ab/cd", "app.js",
				"sub/.git/x", "worktree/.git",
			},
			notice: noticeEnvOnly,
		},
		{
			name:  "all of them",
			lines: "!.git/\n!.env\n!.env.local\n!.env.*.local\n",
			paths: []string{
				".env", ".env.example", ".env.local", ".env.production", ".envrc", ".git/config", ".git/objects/ab/cd",
				"a/.env.production.local", "app.js", "pkg/api/.env", "sub/.git/x", "worktree/.git",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeTree(t, root, withVcrignore(secretsTree(), tc.lines))

			paths, warn := mustList(t, root)

			assert.Equal(t, tc.paths, paths)
			assert.Equal(t, tc.notice, warn)
		})
	}
}

func TestNoticeNamesOnlyTheGroupsThatWereHit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".git/config": "x", ".git/HEAD": "x", "sub/.git/x": "x", "app.js": "x"})

	_, warn := mustList(t, root)

	assert.Equal(t, noticeGitOnly, warn)
}

func TestNoticeOnlyWhenSomethingWasLeftOut(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"app.js": "x", ".env.production": "x"})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{".env.production", "app.js"}, paths)
	assert.Empty(t, warn)
}

func TestUserListingTheSameFilesSuppressesTheNotice(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".vcrignore": ".env\n", ".env": "x", "app.js": "x"})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"app.js"}, paths)
	assert.Empty(t, warn)
}

func TestDefaultIgnoresAreGitHistoryAndLocalEnvironmentFiles(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{".git/", ".env", ".env.local", ".env.*.local"}, DefaultIgnores)
	assert.Equal(t, []string{".jfs.config", ".jfs.accesslog", ".jfs.stats", ".vcrignore"}, AlwaysSkipped)
}
