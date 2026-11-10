//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// internalRefSamples returns one sample of each kind of internal reference,
// assembled from pieces so that none of them appears literally in the repository.
func internalRefSamples() map[string]string {
	return map[string]string{
		"ticket key":       "ABC" + "-" + "123",
		"security id":      "SEC" + "-" + "6",
		"task id":          "T" + "04",
		"research fact id": "F" + "." + "A1",
		"decision id":      "D" + "12" + "b",
		"note id":          "N" + "3",
		"hotfix id":        "H" + "3",
		"change id":        "C" + "-" + "7",
		"row reference":    "row " + "14",
	}
}

func TestLoadFullExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "example", "deploy-example.yaml"))
	require.NoError(t, err)
	sc, err := ParseScenario("deploy-example", raw)
	require.NoError(t, err)

	assert.Equal(t, "deploy-example", sc.ID)
	assert.Equal(t, "deploy when the first build connection drops", sc.Description)
	assert.Equal(t, "deploy", sc.Group)
	assert.Equal(t, []string{"deploy", "--instance-name", "dev"}, sc.Args)
	assert.Equal(t, "work/project", sc.Cwd)
	assert.False(t, sc.TTY)
	require.NotNil(t, sc.Stdin)
	assert.Equal(t, "", *sc.Stdin)
	assert.Equal(t, "release", sc.Build)
	assert.Equal(t, "stamped", sc.Exe)
	assert.Empty(t, sc.Requires)
	assert.False(t, sc.RequiresHotfix)
	assert.Empty(t, sc.HoldPorts)
	assert.Empty(t, sc.Env)
	assert.Equal(t, "default", sc.Config)
	assert.Contains(t, sc.ConfigFile, "graphql_endpoint = ${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql")
	require.Len(t, sc.Files, 6)
	assert.Equal(t, "work/project/vcr.yml", sc.Files[0].Path)
	assert.Contains(t, *sc.Files[0].Content, "application-id: 00000000-0000-4000-8000-000000000601")
	require.NotNil(t, sc.Files[1].Mode)
	assert.Equal(t, 0o644, *sc.Files[1].Mode)
	assert.Equal(t, "testdata/corpora/corpus.ini", sc.Files[2].From)
	assert.Equal(t, "index.js", sc.Files[3].Link)
	assert.Equal(t, "fifo", sc.Files[4].Type)
	assert.Equal(t, "dir", sc.Files[5].Type)
	assert.Equal(t, "2026-10-10T10:00:00Z", sc.Files[5].Mtime)
	assert.NotZero(t, sc.Fixtures.Kind, "fixtures are kept as a node")
	assert.Empty(t, sc.Steps)
	assert.Equal(t, []string{"euw1 GET /regions/aws.euw1", "euw1 GET /projects/my-project"}, sc.ExpectReads)
	assert.Equal(t, []string{"deployed-banner", "deployed-host-address"}, sc.Scrape)
	assert.Empty(t, sc.ScrapeExcept)
	assert.Empty(t, sc.FencedHost)
	assert.Empty(t, sc.FencedHostBaseline)
	assert.Equal(t, 60*time.Second, sc.Timeout)
}

func TestScenarioDefaults(t *testing.T) {
	sc, err := ParseScenario("version-x", []byte("description: plain\nargs: [version]\n"))
	require.NoError(t, err)
	assert.Equal(t, "work", sc.Cwd)
	assert.Equal(t, "release", sc.Build)
	assert.Equal(t, "stamped", sc.Exe)
	assert.Equal(t, "default", sc.Config)
	assert.Nil(t, sc.Stdin)
	assert.Equal(t, 60*time.Second, sc.Timeout)
}

func TestScenarioValidation(t *testing.T) {
	base := "description: a clean description\nargs: [version]\n"
	cases := []struct{ name, id, text, wantErr string }{
		{"clean", "version-x", base, ""},
		{"unknown key", "version-x", base + "bogus: 1\n", "bogus"},
		{"missing description", "version-x", "args: []\n", "description"},
		{"missing args", "version-x", "description: x y\n", "args"},
		{"unknown group", "version-x", base + "group: nope\n", "group"},
		{"two keys in one step", "version-x", base + "steps:\n  - {send: a, wait-stdout: b}\n", "exactly one"},
		{"no key in a step", "version-x", base + "steps:\n  - {timeout: 1s}\n", "exactly one"},
		{"tty with stdin", "version-x", base + "tty: true\nstdin: hi\n", "stdin"},
		{"fenced host with reads", "app-x", base + "fenced-host: example.test\nexpect-reads: [euw1 GET /regions]\n", "fenced-host"},
		{"fenced host with empty reads", "app-x", base + "fenced-host: example.test\nexpect-reads: []\n", ""},
		{"missing expect-reads in app", "app-x", base, "expect-reads"},
		{"expect-reads present in app", "app-x", base + "expect-reads: []\n", ""},
		{"bad expect-reads entry", "app-x", base + "expect-reads: [GET /regions]\n", "expect-reads"},
		{"expect-reads with a POST", "app-x", base + "expect-reads: [euw1 POST /regions]\n", "expect-reads"},
		{"bad requires", "version-x", base + "requires: [windows]\n", "requires"},
		{"bad build", "version-x", base + "build: staging\n", "build"},
		{"bad exe", "version-x", base + "exe: moved\n", "exe"},
		{"bad config", "version-x", base + "config: maybe\n", "config"},
		{"config file with config none", "version-x", base + "config: none\nconfig-file: x\n", "config-file"},
		{"bad timeout", "version-x", base + "timeout: soon\n", "timeout"},
		{"absolute file path", "version-x", base + "files:\n  - {path: /etc/x, content: y}\n", "path"},
		{"escaping file path", "version-x", base + "files:\n  - {path: ../x, content: y}\n", "path"},
		{"file with two kinds", "version-x", base + "files:\n  - {path: work/x, content: y, link: z}\n", "exactly one"},
		{"file with no kind", "version-x", base + "files:\n  - {path: work/x}\n", "exactly one"},
		{"bad file type", "version-x", base + "files:\n  - {path: work/x, type: socket}\n", "type"},
		{"bad signal", "version-x", base + "steps:\n  - {signal: SIGHUP}\n", "signal"},
		{"bad wait-peer", "version-x", base + "steps:\n  - {wait-peer: maybe}\n", "wait-peer"},
		{"bad wait-request", "version-x", base + "steps:\n  - {wait-request: POST}\n", "wait-request"},
		{"bad duration in a step", "version-x", base + "steps:\n  - {sleep: later}\n", "sleep"},
		{"bad hold port name", "version-x", base + "hold-ports: [\"a:b\"]\n", "hold-ports"},
	}
	for kind, id := range internalRefSamples() {
		cases = append(cases, struct{ name, id, text, wantErr string }{
			"description with " + kind, "version-x", "description: see " + id + " for more\nargs: []\n", "internal reference",
		})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseScenario(c.id, []byte(c.text))
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

func TestScenarioGroupRules(t *testing.T) {
	base := "description: a clean description\nargs: []\n"
	reads := "expect-reads: []\n"
	cases := []struct{ name, id, text, want, wantErr string }{
		{"prefix is a group", "deploy-x", base + reads, "deploy", ""},
		{"id without a dash", "version", base, "version", ""},
		{"security without group", "security-x", base, "", "group"},
		{"unknown group key", "deploy-x", base + reads + "group: nope\n", "", "group"},
		{"security with group", "security-x", base + reads + "group: app\n", "app", ""},
		{"group differs from a valid prefix", "deploy-x", base + reads + "group: app\n", "app", ""},
		{"unknown prefix without group", "frobnicate-x", base, "", "group"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc, err := ParseScenario(c.id, []byte(c.text))
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, sc.Group)
		})
	}
}

func TestStepKindsDecode(t *testing.T) {
	text := "description: every step\nargs: []\nsteps:\n" +
		"  - {wait-stdout: ready}\n  - {wait-stderr: warn}\n  - {expect: screen}\n" +
		"  - {wait-request: \"POST /debug/services\", timeout: 5s}\n  - {wait-peer: connected}\n" +
		"  - {peer-send: {id: a1, operation: execute, method: GET, route: /hello, query: {k: [v]}, headers: {h: [x]}, payload: body}}\n" +
		"  - {wait-peer-response: a1}\n  - {peer-close: {code: 1008, text: bye}}\n" +
		"  - {local-call: {method: GET, url: \"http://127.0.0.1:1/x\", headers: {a: b}, body: z}}\n" +
		"  - {public-call: {method: POST, path: /echo, headers: {a: b}, body: z}}\n" +
		"  - {send: \"yes\\n\"}\n  - {close-stdin: true}\n  - {signal: SIGINT}\n  - {sleep: 1s}\n  - {wait-exit: 3s}\n"
	sc, err := ParseScenario("version-steps", []byte(text))
	require.NoError(t, err)
	var kinds []string
	for _, s := range sc.Steps {
		kinds = append(kinds, s.Kind())
	}
	assert.Equal(t, []string{
		"wait-stdout", "wait-stderr", "expect", "wait-request", "wait-peer", "peer-send", "wait-peer-response",
		"peer-close", "local-call", "public-call", "send", "close-stdin", "signal", "sleep", "wait-exit",
	}, kinds)
	assert.Equal(t, 5*time.Second, sc.Steps[3].Timeout)
	require.NotNil(t, sc.Steps[5].PeerSend)
	assert.Equal(t, "a1", sc.Steps[5].PeerSend.ID)
	assert.Equal(t, map[string][]string{"k": {"v"}}, sc.Steps[5].PeerSend.Query)
	assert.Equal(t, 1008, sc.Steps[7].PeerClose.Code)
	assert.Equal(t, "/echo", sc.Steps[9].PublicCall.Path)
	assert.Equal(t, "yes\n", *sc.Steps[10].Send)
	assert.Equal(t, time.Second, sc.Steps[13].Sleep)
}

func TestLoadScenariosReadsAFolderInIDOrder(t *testing.T) {
	dir := t.TempDir()
	body := []byte("description: a clean description\nargs: []\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version-b.yaml"), body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version-a.yaml"), body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644))
	got, err := LoadScenarios(dir)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "version-a", got[0].ID)
	assert.Equal(t, "version-b", got[1].ID)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "version-c.yaml"), []byte("bogus: 1\n"), 0o644))
	_, err = LoadScenarios(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version-c")

	none, err := LoadScenarios(filepath.Join(dir, "missing"))
	require.NoError(t, err)
	assert.Empty(t, none)
}
