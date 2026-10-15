//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	protUUID = "11111111-2222-4333-8444-555555555555"
	randUUID = "99999999-8888-4777-8666-555555555555"
	protTime = "2026-10-10T10:00:00Z"
)

func testLayout() Layout {
	return Layout{
		Root: "/tmp/x/s", Home: "/tmp/x/s/home", Work: "/tmp/x/s/work",
		Tmp: "/tmp/x/s/tmp", Outside: "/tmp/x/s/outside", Bin: "/tmp/x/s/bin",
	}
}

func testNormaliser() Normaliser {
	return NewNormaliser(testLayout(), "/tmp/x/testapp", 5000, 6000, map[string]int{"app": 7000},
		map[string]bool{protUUID: true, protTime: true})
}

type ruleCase struct{ name, in, want string }

func runRule(t *testing.T, fn func(string) string, cases []ruleCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, fn(c.in)) })
	}
}

func TestNormaliseRules(t *testing.T) {
	n := testNormaliser()
	t.Run("1 sandbox paths", func(t *testing.T) {
		runRule(t, n.Paths, []ruleCase{
			{"home", "read /tmp/x/s/home/.vcr-cli failed", "read <HOME>/.vcr-cli failed"},
			{"longest first", "/tmp/x/s/work/a /tmp/x/s/tmp/b /tmp/x/s/outside /tmp/x/s/bin/vcr", "<WORK>/a <TMP>/b <OUTSIDE> <BIN>/vcr"},
			{"root", "under /tmp/x/s/other and /tmp/x/s", "under <SANDBOX>/other and <SANDBOX>"},
			{"helper", "run /tmp/x/testapp now", "run <TESTAPP> now"},
			{"sibling directory is not a prefix match", "/tmp/x/sandbox/home /tmp/x/s2/work", "/tmp/x/sandbox/home /tmp/x/s2/work"},
			{"unrelated", "/usr/local/bin/vcr", "/usr/local/bin/vcr"},
		})
	})
	t.Run("2 fake addresses", func(t *testing.T) {
		runRule(t, n.Addresses, []ruleCase{
			{"http", "GET http://127.0.0.1:5000/v1/x", "GET <FAKE>/v1/x"},
			{"ws", "ws://127.0.0.1:5000/watch", "<FAKE_WS>/watch"},
			{"host", "dial 127.0.0.1:5000: refused", "dial <FAKE_HOST>: refused"},
			{"other http", "http://localhost:6000/p", "<OTHER>/p"},
			{"other ws", "ws://localhost:6000/tunnel", "<OTHER_WS>/tunnel"},
			{"other host", "Application Host: localhost:6000", "Application Host: <OTHER_HOST>"},
			{"other same host", "127.0.0.1:6000", "<OTHER_SAME_HOST>"},
			{"named port", "listening on :7000 and 127.0.0.1:7000", "listening on :<PORT:app> and 127.0.0.1:<PORT:app>"},
			{"longer port is not a prefix match", "http://127.0.0.1:50001/x 127.0.0.1:9 port 70001", "http://127.0.0.1:50001/x 127.0.0.1:9 port 70001"},
			{"other host names", "http://example.com:5000", "http://example.com:5000"},
		})
	})
	t.Run("3 stderr log lines", func(t *testing.T) {
		both := func(s string) string { return MaskLogTimes(DropRestyWarnings(s)) }
		runRule(t, both, []ruleCase{
			{"warning line dropped", "2026/10/10 10:00:00 WARN RESTY basic auth over http\nreal line\n", "real line\n"},
			{"warning with fraction dropped", "2026/10/10 10:00:00.123456 WARN RESTY x\n", ""},
			{"other timestamp masked", "2026/10/10 10:00:01.5 ERROR boom\n", "<time> ERROR boom\n"},
			{"dash dates are left to the identifier rule", "2026-10-10 10:00:00 ok\n", "2026-10-10 10:00:00 ok\n"},
			{"other warn is kept", "WARN something else\n", "WARN something else\n"},
		})
		assert.Equal(t, "2026/10/10 10:00:00 hi\n", n.Stream("2026/10/10 10:00:00 hi\n", false), "stdout keeps log times")
		assert.Equal(t, "<time> hi\n", n.Stream("2026/10/10 10:00:00 hi\n", true), "stderr masks log times")
	})
	t.Run("4 identifiers", func(t *testing.T) {
		runRule(t, n.Identifiers, []ruleCase{
			{"random uuid", "id " + randUUID + " ok", "id <uuid> ok"},
			{"protected uuid", "id " + protUUID, "id " + protUUID},
			{"random timestamp", "at 2026-11-11T11:11:11.123456+02:00", "at <time>"},
			{"protected timestamp", "at " + protTime, "at " + protTime},
			{"upper case uuid", "ID " + "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE", "ID <uuid>"},
			{"date only", "on 2026-10-10 and 10:00:00 and v2.6.1", "on 2026-10-10 and 10:00:00 and v2.6.1"},
			{"short hex", "abcdef12-1234 deadbeef", "abcdef12-1234 deadbeef"},
		})
	})
	t.Run("5 platform", func(t *testing.T) {
		pair, space := runtime.GOOS+"_"+runtime.GOARCH, runtime.GOOS+" "+runtime.GOARCH
		runRule(t, Platform, []ruleCase{
			{"asset name", "vcr_" + pair + ".tar.gz", "vcr_<os>_<arch>.tar.gz"},
			{"error text", "unsupported platform " + space, "unsupported platform <os> <arch>"},
			{"other platform", "vcr_plan9_riscv.tar.gz plan9 riscv", "vcr_plan9_riscv.tar.gz plan9 riscv"},
		})
	})
	t.Run("6 debug access log", func(t *testing.T) {
		runRule(t, DebugAccessLog, []ruleCase{
			{"ms", "[VCR-debug] 2026/10/10 - 10:00:00 |  200 |       1.234ms | GET  /hello\n", "[VCR-debug] <time> | 200 | <dur> | GET  /hello\n"},
			{"microseconds", "[VCR-debug] 2026/10/10 - 10:00:00 | 404 |      12.5µs | POST   /echo", "[VCR-debug] <time> | 404 | <dur> | POST   /echo"},
			{"second line only", "ready\n[VCR-debug] 2026/01/02 - 03:04:05 | 200 | 3s | GET  /a\n", "ready\n[VCR-debug] <time> | 200 | <dur> | GET  /a\n"},
			{"not an access line", "[VCR-debug] listening\n", "[VCR-debug] listening\n"},
			{"not at line start", "x [VCR-debug] 2026/10/10 - 10:00:00 | 200 | 1ms | GET  /a", "x [VCR-debug] 2026/10/10 - 10:00:00 | 200 | 1ms | GET  /a"},
		})
	})
	t.Run("7 json bodies", func(t *testing.T) {
		runRule(t, CanonicalJSON, []ruleCase{
			{"sorted compact", `{"b": 1, "a": {"z": null, "y": [true, 2.50]}}`, `{"a":{"y":[true,2.50],"z":null},"b":1}`},
			{"big numbers keep their text", `{"n": 12345678901234567890}`, `{"n":12345678901234567890}`},
			{"no html escaping", `{"k": "<a&b>"}`, `{"k":"<a&b>"}`},
			{"not json", "plain text", "plain text"},
			{"trailing garbage", `{"a":1} x`, `{"a":1} x`},
			{"empty", "", ""},
		})
	})
}

func TestBodyAppliesTheStringRulesThenCanonicalJSON(t *testing.T) {
	n := testNormaliser()
	in := `{"id": "` + randUUID + `", "keep": "` + protUUID + `", "url": "http://127.0.0.1:5000/x", "b": 1}`
	want := `{"b":1,"id":"<uuid>","keep":"` + protUUID + `","url":"<FAKE>/x"}`
	assert.Equal(t, want, n.Body(in))
}

func TestPathsNormaliseTheResolvedFormToo(t *testing.T) {
	real := t.TempDir()
	via := filepath.Join(t.TempDir(), "via")
	require.NoError(t, os.Symlink(real, via))
	l, err := CreateSandbox(via)
	require.NoError(t, err)
	n := NewNormaliser(l, "", 0, 0, nil, nil)
	resolved, err := filepath.EvalSymlinks(filepath.Join(real, "s", "work"))
	require.NoError(t, err)
	assert.Equal(t, "boom <WORK>/x", n.Paths("boom "+resolved+"/x"))
	assert.Equal(t, "boom <WORK>/x", n.Paths("boom "+l.Work+"/x"))
}

func TestEscapeRoundTrip(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "plain text\n", "plain text\n"},
		{"esc", "\x1b[35mℹ\x1b[0m", `\x1b[35mℹ\x1b[0m`},
		{"nul", "a\x00b", `a\x00b`},
		{"invalid utf8", "bad\xff\xfeend", `bad\xff\xfeend`},
		{"backslash", `back\slash`, `back\\slash`},
		{"literal escape text", `\x41`, `\\x41`},
		{"tab and newline stay", "a\tb\nc", "a\tb\nc"},
		{"carriage return", "a\r\nb", `a\x0d` + "\nb"},
		{"del", "\x7f", `\x7f`},
		{"vertical tab", "\x0b", `\x0b`},
		{"unicode", "┌─┐ → ℹ", "┌─┐ → ℹ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Escape(c.in))
			back, err := Unescape(Escape(c.in))
			require.NoError(t, err)
			assert.Equal(t, c.in, back)
		})
	}
	_, err := Unescape(`bad \q escape`)
	assert.Error(t, err)
	_, err = Unescape(`short \x1`)
	assert.Error(t, err)
}

func TestProtectedSetIsExactlyFixturesArgsFilesAndConfig(t *testing.T) {
	const (
		uFix   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		uArg   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		uFile  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		uCfg   = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		uOther = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		uStep  = "ffffffff-ffff-4fff-8fff-ffffffffffff"
		tsFix  = "2026-01-02T03:04:05Z"
		tsOth  = "2026-05-06T07:08:09Z"
	)
	text := "description: protected literals\n" +
		"args: [version, \"" + uArg + "\"]\n" +
		"env: {OTHER: \"" + uOther + "\"}\n" +
		"config-file: |\n  note = " + uCfg + "\n" +
		"files:\n  - path: work/a.txt\n    content: \"" + uFile + "\\n\"\n" +
		"fixtures:\n  secrets: [\"" + uFix + "\", \"x" + uOther + "\"]\n  logs: [{timestamp: " + tsFix + ", message: hi}]\n" +
		"steps:\n  - send: \"" + uStep + "\"\n"
	sc, err := ParseScenario("version-protected", []byte(text))
	require.NoError(t, err)
	n := NewNormaliser(testLayout(), "", 0, 0, nil, ProtectedLiterals(sc, ""))
	for _, kept := range []string{uFix, uArg, uFile, uCfg, tsFix} {
		assert.Equal(t, kept, n.Identifiers(kept), kept)
	}
	assert.Equal(t, "<uuid>", n.Identifiers(uOther), "an id in env only is masked")
	assert.Equal(t, "<uuid>", n.Identifiers(uStep), "an id in a step only is masked")
	assert.Equal(t, "<time>", n.Identifiers(tsOth))
}
