//go:build !windows

package contract

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func identity(s string) string { return s }

func mustScenario(t *testing.T, id, extra string) Scenario {
	t.Helper()
	sc, err := ParseScenario(id, []byte("description: a clean description\nargs: []\n"+extra))
	require.NoError(t, err)
	return sc
}

func sumOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestSandboxLayoutAndDefaultConfig(t *testing.T) {
	base := t.TempDir()
	l, err := CreateSandbox(base)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "s"), l.Root)
	for name, dir := range map[string]string{"home": l.Home, "work": l.Work, "tmp": l.Tmp, "outside": l.Outside, "bin": l.Bin} {
		assert.Equal(t, filepath.Join(l.Root, name), dir)
		st, err := os.Stat(dir)
		require.NoError(t, err, name)
		assert.True(t, st.IsDir(), name)
	}

	require.NoError(t, l.WriteConfig(mustScenario(t, "version-x", ""), identity))
	got, err := os.ReadFile(filepath.Join(l.Home, ".vcr-cli"))
	require.NoError(t, err)
	want := "graphql_endpoint = ${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql\n" +
		"default_region   = aws.euw1\n" +
		"endpoint         = ${FAKE}/v1/vcr/euw1/api/deployment\n\n" +
		"[credentials]\n" +
		"api_key    = abc123\n" +
		"api_secret = s3cret-for-tests\n"
	assert.Equal(t, want, string(got))
	st, err := os.Stat(filepath.Join(l.Home, ".vcr-cli"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}

func TestConfigFileAndConfigNone(t *testing.T) {
	expand := strings.NewReplacer("${FAKE}", "http://127.0.0.1:1").Replace
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, l.WriteConfig(mustScenario(t, "version-x", "config-file: |\n  endpoint = ${FAKE}/x\n"), expand))
	got, err := os.ReadFile(filepath.Join(l.Home, ".vcr-cli"))
	require.NoError(t, err)
	assert.Equal(t, "endpoint = http://127.0.0.1:1/x\n", string(got))

	l2, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, l2.WriteConfig(mustScenario(t, "version-x", "config: none\n"), expand))
	_, err = os.Stat(filepath.Join(l2.Home, ".vcr-cli"))
	assert.True(t, os.IsNotExist(err))
}

func TestFilesMaterialise(t *testing.T) {
	contract := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(contract, "testdata", "corpora"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contract, "testdata", "corpora", "in.txt"), []byte("corpus\n"), 0o600))
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	sc := mustScenario(t, "version-x", `files:
  - {path: work/deep/er/a.txt, content: "alpha\n"}
  - {path: work/private.txt, content: "secret\n", mode: 0600}
  - {path: work/link, link: a.txt}
  - {path: work/pipe, type: fifo}
  - {path: work/empty, type: dir, mtime: 2026-10-10T10:00:00Z}
  - {path: work/old.txt, content: "old\n", mtime: 2026-01-02T03:04:05Z}
  - {path: work/copy.txt, from: testdata/corpora/in.txt}
`)
	require.NoError(t, l.Materialise(sc.Files, contract, identity))

	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(l.Root, p))
		require.NoError(t, err)
		return string(b)
	}
	assert.Equal(t, "alpha\n", read("work/deep/er/a.txt"))
	st, err := os.Stat(filepath.Join(l.Root, "work/deep/er/a.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), st.Mode().Perm())
	st, err = os.Stat(filepath.Join(l.Root, "work/deep"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())
	st, err = os.Stat(filepath.Join(l.Root, "work/private.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	target, err := os.Readlink(filepath.Join(l.Root, "work/link"))
	require.NoError(t, err)
	assert.Equal(t, "a.txt", target)
	st, err = os.Stat(filepath.Join(l.Root, "work/pipe"))
	require.NoError(t, err)
	assert.NotZero(t, st.Mode()&os.ModeNamedPipe)
	st, err = os.Stat(filepath.Join(l.Root, "work/empty"))
	require.NoError(t, err)
	assert.True(t, st.IsDir())
	assert.True(t, st.ModTime().Equal(time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)))
	st, err = os.Stat(filepath.Join(l.Root, "work/old.txt"))
	require.NoError(t, err)
	assert.True(t, st.ModTime().Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	assert.Equal(t, "corpus\n", read("work/copy.txt"))
}

func TestFromFilesAreExpandedAsText(t *testing.T) {
	contract := t.TempDir()
	corpora := filepath.Join(contract, "testdata", "corpora")
	require.NoError(t, os.MkdirAll(corpora, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(corpora, "cfg.ini"), []byte("url = ${FAKE}/x\nhome = ${HOME}\n"), 0o644))
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, err := zw.Write([]byte("archive ${FAKE} ${HOME}"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(filepath.Join(corpora, "a.tgz"), gz.Bytes(), 0o644))

	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	expand := strings.NewReplacer("${FAKE}", "http://127.0.0.1:7", "${HOME}", "/h").Replace
	sc := mustScenario(t, "version-x", "files:\n  - {path: work/cfg.ini, from: testdata/corpora/cfg.ini}\n  - {path: work/a.tgz, from: testdata/corpora/a.tgz}\n")
	require.NoError(t, l.Materialise(sc.Files, contract, expand))

	text, err := os.ReadFile(filepath.Join(l.Work, "cfg.ini"))
	require.NoError(t, err)
	assert.Equal(t, "url = http://127.0.0.1:7/x\nhome = /h\n", string(text))
	bin, err := os.ReadFile(filepath.Join(l.Work, "a.tgz"))
	require.NoError(t, err)
	assert.Equal(t, gz.Bytes(), bin, "a file that is not text is copied byte for byte")
}

func TestMaterialiseRejectsAFromFileOutsideTheContractFolder(t *testing.T) {
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	sc := mustScenario(t, "version-x", "files:\n  - {path: work/x, from: ../../etc/passwd}\n")
	assert.Error(t, l.Materialise(sc.Files, t.TempDir(), identity))
}

func TestSnapshotDiff(t *testing.T) {
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	put := func(rel, content string, mode os.FileMode) {
		p := filepath.Join(l.Root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), mode))
		require.NoError(t, os.Chmod(p, mode))
	}
	put("work/keep.txt", "same\n", 0o644)
	put("work/mod.txt", "one\n", 0o644)
	put("work/mode.txt", "same\n", 0o644)
	put("work/del.txt", "gone\n", 0o644)
	require.NoError(t, os.MkdirAll(filepath.Join(l.Root, "work/deldir"), 0o755))
	before, err := TakeSnapshot(l.Root)
	require.NoError(t, err)

	put("work/mod.txt", "two\n", 0o644)
	require.NoError(t, os.Chmod(filepath.Join(l.Root, "work/mode.txt"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(l.Root, "work/del.txt")))
	require.NoError(t, os.Remove(filepath.Join(l.Root, "work/deldir")))
	put("work/new.txt", "hello\nworld\n", 0o644)
	require.NoError(t, os.Mkdir(filepath.Join(l.Root, "work/sub"), 0o755))
	require.NoError(t, os.Symlink("target", filepath.Join(l.Root, "work/link")))
	big := strings.Repeat("x", 5000)
	put("work/big.txt", big, 0o644)
	put("work/blob.bin", "\xff\xfe\x00data", 0o755)
	after, err := TakeSnapshot(l.Root)
	require.NoError(t, err)

	got := RenderFiles(DiffSnapshots(before, after, NewNormaliser(l, "", 0, 0, nil, nil)))
	want := "+ work/big.txt mode=0644 size=5000 sha256=" + sumOf(big) + "\n" +
		"+ work/blob.bin mode=0755 size=7 sha256=" + sumOf("\xff\xfe\x00data") + "\n" +
		"+ work/link -> target\n" +
		"~ work/mod.txt mode=0644 size=4 sha256=" + sumOf("two\n") + "\n  | two\n" +
		"~ work/mode.txt mode=0644->0600\n" +
		"+ work/new.txt mode=0644 size=12 sha256=" + sumOf("hello\nworld\n") + "\n  | hello\n  | world\n" +
		"- work/del.txt\n" +
		"- work/deldir/\n" +
		"+ work/sub/ mode=0755\n"
	// The entries come back sorted by path, so compare as sorted blocks.
	assert.ElementsMatch(t, splitFileBlocks(want), splitFileBlocks(got))
	paths := []string{}
	for _, c := range DiffSnapshots(before, after, Normaliser{}) {
		paths = append(paths, c.Path)
	}
	assert.IsIncreasing(t, paths)
	assert.NotContains(t, got, "keep.txt")
	assert.NotContains(t, got, "work/ ", "a parent directory whose time changed is not a change")
}

func splitFileBlocks(s string) []string {
	var out []string
	for _, l := range strings.SplitAfter(s, "\n") {
		switch {
		case l == "":
		case strings.HasPrefix(l, "  | ") && len(out) > 0:
			out[len(out)-1] += l
		default:
			out = append(out, l)
		}
	}
	return out
}

func TestSnapshotDigestsNormalisedText(t *testing.T) {
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	before, err := TakeSnapshot(l.Root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(l.Work, "conf"), []byte("home = "+l.Home+"/x\n"), 0o644))
	after, err := TakeSnapshot(l.Root)
	require.NoError(t, err)

	changes := DiffSnapshots(before, after, NewNormaliser(l, "", 0, 0, nil, nil))
	require.Len(t, changes, 1)
	assert.Equal(t, "home = <HOME>/x\n", changes[0].Text)
	assert.Equal(t, int64(len("home = <HOME>/x\n")), changes[0].Size)
	assert.Equal(t, sumOf("home = <HOME>/x\n"), changes[0].SHA)
}

func TestSnapshotSeesAChangedFifoTypeAndModeOfADirectory(t *testing.T) {
	l, err := CreateSandbox(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(l.Work, "d"), 0o755))
	require.NoError(t, syscall.Mkfifo(filepath.Join(l.Work, "p"), 0o644))
	before, err := TakeSnapshot(l.Root)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(l.Work, "d"), 0o700))
	after, err := TakeSnapshot(l.Root)
	require.NoError(t, err)
	got := RenderFiles(DiffSnapshots(before, after, Normaliser{}))
	assert.Equal(t, "~ work/d/ mode=0755->0700\n", got)
}
