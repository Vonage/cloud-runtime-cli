//go:build !windows

package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	uuidPattern = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`
	timePattern = `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`
)

var (
	identifierPattern = regexp.MustCompile(uuidPattern + "|" + timePattern)
	restyWarning      = regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? WARN RESTY .*\n?`)
	logTime           = regexp.MustCompile(`\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?`)
	debugAccessLine   = regexp.MustCompile(`(?m)^\[VCR-debug\] \d{4}/\d{2}/\d{2} - \d{2}:\d{2}:\d{2} \| +(\d+) \| +[0-9.]+(?:ns|µs|us|ms|s) +\| `)
)

// Layout is where a scenario's sandbox lives.
type Layout struct{ Root, Home, Work, Tmp, Outside, Bin string }

type replacement struct {
	from, to string
	digits   bool // the match must not touch a digit (ports); else it must not touch a name byte (paths)
}

// Normaliser applies the normalisation rules of one run: the sandbox, the
// addresses and ports of the fake, and the ids that must stay verbatim. The
// zero value changes only what needs no run-specific knowledge.
type Normaliser struct {
	paths, addrs []replacement
	protected    map[string]bool
}

// NewNormaliser prepares the rules for a run. Ports of 0 are left out.
func NewNormaliser(l Layout, testapp string, fakePort, otherPort int, ports map[string]int, protected map[string]bool) Normaliser {
	n := Normaliser{protected: protected}
	labelled := []struct{ path, label string }{
		{l.Root, "<SANDBOX>"}, {l.Home, "<HOME>"}, {l.Work, "<WORK>"}, {l.Tmp, "<TMP>"},
		{l.Outside, "<OUTSIDE>"}, {l.Bin, "<BIN>"}, {testapp, "<TESTAPP>"},
	}
	for _, p := range labelled {
		if p.path == "" {
			continue
		}
		n.paths = append(n.paths, replacement{from: p.path, to: p.label})
		if resolved, err := filepath.EvalSymlinks(p.path); err == nil && resolved != p.path {
			n.paths = append(n.paths, replacement{from: resolved, to: p.label})
		}
	}
	sort.SliceStable(n.paths, func(i, j int) bool { return len(n.paths[i].from) > len(n.paths[j].from) })
	n.addrs = addressReplacements(fakePort, otherPort, ports)
	return n
}

func addressReplacements(fakePort, otherPort int, ports map[string]int) []replacement {
	var out []replacement
	add := func(from, to string) { out = append(out, replacement{from: from, to: to, digits: true}) }
	if fakePort > 0 {
		p := strconv.Itoa(fakePort)
		add("http://127.0.0.1:"+p, "<FAKE>")
		add("ws://127.0.0.1:"+p, "<FAKE_WS>")
		add("127.0.0.1:"+p, "<FAKE_HOST>")
	}
	if otherPort > 0 {
		p := strconv.Itoa(otherPort)
		add("http://localhost:"+p, "<OTHER>")
		add("ws://localhost:"+p, "<OTHER_WS>")
		add("localhost:"+p, "<OTHER_HOST>")
		add("127.0.0.1:"+p, "<OTHER_SAME_HOST>")
	}
	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add(strconv.Itoa(ports[name]), "<PORT:"+name+">")
	}
	return out
}

func isNameByte(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// replaceBounded replaces the occurrences of from that do not run into a longer
// name (or number): the byte after the match, and for numbers also the one
// before it, must not be part of one.
func replaceBounded(s string, r replacement) string {
	if r.from == "" || !strings.Contains(s, r.from) {
		return s
	}
	touches := isNameByte
	if r.digits {
		touches = isDigitByte
	}
	numeric := r.digits && isDigitByte(r.from[0]) && isDigitByte(r.from[len(r.from)-1])
	var b strings.Builder
	for {
		i := strings.Index(s, r.from)
		if i < 0 {
			break
		}
		end := i + len(r.from)
		ok := end == len(s) || !touches(s[end])
		if numeric && i > 0 && isDigitByte(s[i-1]) {
			ok = false
		}
		if ok {
			b.WriteString(s[:i] + r.to)
			s = s[end:]
			continue
		}
		b.WriteString(s[:i+1])
		s = s[i+1:]
	}
	b.WriteString(s)
	return b.String()
}

// Paths is rule 1: the sandbox and helper paths, also in their symlink-resolved form.
func (n Normaliser) Paths(s string) string {
	for _, r := range n.paths {
		s = replaceBounded(s, r)
	}
	return s
}

// Addresses is rule 2: the fake's addresses and the named ports.
func (n Normaliser) Addresses(s string) string {
	for _, r := range n.addrs {
		s = replaceBounded(s, r)
	}
	return s
}

// DropRestyWarnings is the first half of rule 3: the HTTP library's warning lines.
func DropRestyWarnings(s string) string { return restyWarning.ReplaceAllString(s, "") }

// MaskLogTimes is the second half of rule 3: the times of log lines.
func MaskLogTimes(s string) string { return logTime.ReplaceAllString(s, "<time>") }

// Identifiers is rule 4: every UUID and RFC 3339 timestamp that is not protected.
func (n Normaliser) Identifiers(s string) string {
	return identifierPattern.ReplaceAllStringFunc(s, func(m string) string {
		switch {
		case n.protected[m]:
			return m
		case len(m) == 36 && m[8] == '-':
			return "<uuid>"
		}
		return "<time>"
	})
}

// Platform is rule 5: the host's <os>_<arch> and "<os> <arch>".
func Platform(s string) string {
	s = strings.ReplaceAll(s, runtime.GOOS+"_"+runtime.GOARCH, "<os>_<arch>")
	return strings.ReplaceAll(s, runtime.GOOS+" "+runtime.GOARCH, "<os> <arch>")
}

// DebugAccessLog is rule 6: the time and latency of the debug access log.
func DebugAccessLog(s string) string {
	return debugAccessLine.ReplaceAllString(s, "[VCR-debug] <time> | $1 | <dur> | ")
}

// CanonicalJSON is rule 7: a JSON text re-encoded compactly with sorted keys
// and numbers as written; anything that is not exactly one JSON value stays.
func CanonicalJSON(s string) string {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return s
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return s
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return s
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// Stream applies rules 1 to 6 to one output stream; rule 3 only to stderr.
func (n Normaliser) Stream(s string, stderr bool) string {
	s = n.Addresses(n.Paths(s))
	if stderr {
		s = MaskLogTimes(DropRestyWarnings(s))
	}
	return DebugAccessLog(Platform(n.Identifiers(s)))
}

// Text applies the rules that every text gets: a file's content, a request line.
func (n Normaliser) Text(s string) string { return n.Stream(s, false) }

// Body applies rules 1 to 7 to a request body.
func (n Normaliser) Body(s string) string { return CanonicalJSON(n.Text(s)) }

// ProtectedLiterals returns the UUIDs and timestamps that stay verbatim: the
// whole tokens that occur in the scenario's fixtures, args, files and
// config-file, and nowhere else. corpora is the folder that a "from" file is
// read from; "" leaves those files out.
func ProtectedLiterals(sc Scenario, corpora string) map[string]bool {
	var texts []string
	collectScalars(&sc.Fixtures, &texts)
	texts = append(texts, sc.Args...)
	texts = append(texts, sc.ConfigFile)
	for _, f := range sc.Files {
		if f.Content != nil {
			texts = append(texts, *f.Content)
		}
		if f.From != "" && corpora != "" {
			if b, err := os.ReadFile(filepath.Join(corpora, filepath.FromSlash(f.From))); err == nil && utf8.Valid(b) {
				texts = append(texts, string(b))
			}
		}
	}
	out := map[string]bool{}
	for _, t := range texts {
		for _, loc := range identifierPattern.FindAllStringIndex(t, -1) {
			start, end := loc[0], loc[1]
			if (start == 0 || !isNameByte(t[start-1])) && (end == len(t) || !isNameByte(t[end])) {
				out[t[start:end]] = true
			}
		}
	}
	return out
}

// collectScalars gathers the values of a YAML tree: mapping keys are structure.
func collectScalars(n *yaml.Node, out *[]string) {
	switch n.Kind {
	case yaml.ScalarNode:
		*out = append(*out, n.Value)
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			collectScalars(n.Content[i], out)
		}
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			collectScalars(c, out)
		}
	case yaml.AliasNode:
		if n.Alias != nil {
			collectScalars(n.Alias, out)
		}
	}
}

// freePort returns a TCP port on 127.0.0.1 that was free a moment ago.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
