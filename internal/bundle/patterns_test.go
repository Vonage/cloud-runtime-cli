package bundle

import (
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// patternTree holds the names the .vcrignore forms below act on.
var patternTree = []string{
	"index.js", "app.log", "keep.log", "notes.tmp", "escaped-space.txt ", "leading-spaces.txt", "root-only.txt",
	"sub/root-only.txt", "sub/deep/trace.log", "sub/readme.md", "build/out.js", "src/build/x.js",
	"node_modules/pkg/index.js", "src/node_modules/y.js", "temp/a.txt", "x.bak", "sub/y.bak", "cache/blob.bin",
	"other/cache", "docs/readme.md", "docs/guide.md", "generated/g.js", "src/generated/h.js", "logs/a/b/c.txt",
	"maps/app.js.map", "#important.txt", "!bang.txt", "#not-a-pattern.txt", "src/main.js",
}

func TestVcrignorePatternForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		lines   string
		removed []string
	}{
		{
			name:    "comments and blank lines are skipped",
			lines:   "# ignore the logs\n\n*.log\n\n# and the temporary files\n*.tmp\n",
			removed: []string{"app.log", "keep.log", "notes.tmp", "sub/deep/trace.log"},
		},
		{
			name:    "trailing spaces are dropped and leading spaces too",
			lines:   "*.tmp  \nescaped-space.txt\\ \n  leading-spaces.txt\n",
			removed: []string{"notes.tmp", "leading-spaces.txt"},
		},
		{
			name:    "double stars match across directories",
			lines:   "**/generated/**\nlogs/**/*.txt\n**/*.map\n",
			removed: []string{"generated/g.js", "src/generated/h.js", "logs/a/b/c.txt", "maps/app.js.map"},
		},
		{
			name:    "a leading slash anchors at the root",
			lines:   "/build\n/root-only.txt\n",
			removed: []string{"root-only.txt", "build/out.js"},
		},
		{
			name:    "a name without a slash matches at any depth",
			lines:   "node_modules\ntemp\n*.bak\n",
			removed: []string{"node_modules/pkg/index.js", "src/node_modules/y.js", "temp/a.txt", "x.bak", "sub/y.bak"},
		},
		{
			name:    "a trailing slash names directories",
			lines:   "cache/\nbuild/\n",
			removed: []string{"build/out.js", "src/build/x.js", "cache/blob.bin"},
		},
		{
			name:    "a bang line brings back what an earlier line left out",
			lines:   "*.log\n!keep.log\ndocs/*\n!docs/readme.md\n",
			removed: []string{"app.log", "sub/deep/trace.log", "docs/guide.md"},
		},
		{
			name:    "a backslash escapes a leading hash or bang",
			lines:   "\\#important.txt\n\\!bang.txt\n#not-a-pattern.txt\n",
			removed: []string{"#important.txt", "!bang.txt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tree := map[string]string{".vcrignore": tc.lines}
			var want []string
			for _, name := range patternTree {
				if runtime.GOOS == "windows" && name == "escaped-space.txt " {
					continue
				}
				tree[name] = "x"
				if !slices.Contains(tc.removed, name) {
					want = append(want, name)
				}
			}
			slices.Sort(want)
			root := t.TempDir()
			writeTree(t, root, tree)

			paths, warn := mustList(t, root)

			assert.Equal(t, want, paths)
			assert.Empty(t, warn)
		})
	}
}
