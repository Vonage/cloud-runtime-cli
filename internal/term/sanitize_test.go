package term

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

type sanitizeCase struct {
	name string
	in   string
	want string
}

var sanitizers = []struct {
	name string
	fn   func(string) string
}{
	{"Sanitize", Sanitize},
	{"SanitizeStream", SanitizeStream},
}

var directionMarks = []rune{
	0x200e, 0x200f, 0x061c,
	0x202a, 0x202b, 0x202c, 0x202d, 0x202e,
	0x2066, 0x2067, 0x2068, 0x2069,
}

// controlSequenceCases are inputs whose output is the same for both functions.
func controlSequenceCases() []sanitizeCase {
	cases := []sanitizeCase{
		{"OSC window title", "a\x1b]0;evil title\x07b", "ab"},
		{"OSC clipboard write", "x\x1b]52;c;SGVsbG8=\x07y", "xy"},
		{"OSC hyperlink", "\x1b]8;;http://evil.test\x07click\x1b]8;;\x07", "click"},
		{"OSC ended by ST", "p\x1b]0;t\x1b\\q", "pq"},
		{"clear screen", "a\x1b[2Jb", "ab"},
		{"cursor position report", "a\x1b[6nb", "ab"},
		{"key modifier setting", "a\x1b[>4;2mb", "ab"},
		{"private mode", "a\x1b[?25mb", "ab"},
		{"CSI aborted by a control byte", "a\x1b[3\x071mb", "a1mb"},
		{"8-bit CSI as a rune", "a\u009b2Jb", "ab"},
		{"8-bit CSI as a raw byte", "a\x9b2Jb", "ab"},
		{"lone carriage return", "10%\r20%", "10%20%"},
		{"CRLF", "a\r\nb", "a\nb"},
		{"invalid UTF-8 byte", "a\xffb", "a\uFFFDb"},
		{"truncated UTF-8 sequence", "a\xc3(b", "a\uFFFD(b"},
		{"C0 controls and DEL", "a\x00\x01\x08\x0b\x0c\x7fb", "ab"},
		{"DCS ended by ST", "a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"lone ESC at the end", "abc\x1b", "abc"},
		{"unterminated OSC", "abc\x1b]0;never ends", "abc"},
		{"two-byte escape", "a\x1bcb", "ab"},
		{"escape with an intermediate byte", "a\x1b(Bb", "ab"},
		{"ESC before a newline", "a\x1b\nb", "a\nb"},
		{"CSI too long to keep", "\x1b[" + strings.Repeat("1", 33) + "m", ""},

		{"C1 control as a rune", "a\u0085b", "ab"},
		{"C1 control as a raw byte", "a\x85b", "ab"},
		{"OSC from C1 characters", "a\u009d0;title\u009cb", "ab"},
		{"OSC from raw C1 bytes", "a\x9d0;title\x9cb", "ab"},
		{"OSC from C1 start, ESC backslash end", "a\u009d0;title\x1b\\b", "ab"},
		{"OSC ended by a raw string terminator", "a\x1b]0;t\x9cb", "ab"},
		{"OSC ended by a string terminator rune", "a\x1b]0;t\u009cb", "ab"},
		{"DCS from a C1 character", "a\u0090q\u009cb", "ab"},
		{"SOS from a C1 character", "a\u0098x\u009cb", "ab"},
		{"PM from a C1 character", "a\u009ex\u009cb", "ab"},
		{"APC from a C1 character", "a\u009fx\u009cb", "ab"},
		{"PM and APC after ESC", "a\x1b^x\x1b\\b\x1b_y\x07c", "abc"},
		{"string swallows a newline", "a\x1b]0;t\nb", "a"},
		{"ESC inside a string starts the next sequence", "a\x1b]0;t\x1b[2Jb", "ab"},
		{"ESC inside a string, two-byte escape", "a\x1b]t\x1bxb", "ab"},
		{"cursor movement", "a\x1b[2K\x1b[1G\x1b[2;3H\x1b[1Ab", "ab"},
		{"alternate screen", "a\x1b[?1049hb", "ab"},
		{"CSI with an intermediate byte", "a\x1b[1 qb", "ab"},
		{"SGR with an intermediate byte", "a\x1b[1 mb", "ab"},
		{"SGR with a private marker", "a\x1b[<1mb", "ab"},
		{"ESC then ESC", "a\x1b\x1b[2Jb", "ab"},
		{"CSI aborted by ESC", "a\x1b[31\x1b[2Jb", "ab"},
		{"CSI aborted by DEL", "a\x1b[3\x7f1mb", "a1mb"},
		{"CSI aborted by a letter outside ASCII", "a\x1b[3日1mb", "a日1mb"},
		{"CSI cut at the end", "a\x1b[31", "a"},
		{"CSI introducer cut at the end", "a\x1b[", "a"},
		{"escape with an intermediate byte cut at the end", "a\x1b(", "a"},
		{"escape with an intermediate byte, then a control byte", "a\x1b(\nb", "a\nb"},
		{"ESC before a letter outside ASCII", "a\x1b日b", "a日b"},
		{"ESC before an invalid byte", "a\x1b\xffb", "a\uFFFDb"},
		{"ESC before DEL", "a\x1b\x7fb", "ab"},
		{"ESC before NUL", "a\x1b\x00b", "ab"},
		{"ESC before a direction mark", "a\x1b\u202Eb", "ab"},
		{"ESC before 8-bit CSI", "a\x1b\u009b2Jb", "ab"},
		{"truncated three-byte sequence", "a\xe2\x82b", "a\uFFFDb"},
	}
	for _, r := range directionMarks {
		cases = append(cases, sanitizeCase{fmt.Sprintf("direction mark U+%04X", r), "a" + string(r) + "b", "ab"})
	}
	return cases
}

var ordinaryTexts = []string{
	"a b",
	"a\u3000b",
	"a\u00A0b",
	"\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466",
	"\U0001F1EF\U0001F1F5",
	"日本語",
	"\u00E9",
	"e\u0301",
	"a\u200Cb",
	"\u2764\uFE0F",
	"a\tb\nc",
	"",
}

type colourCase struct {
	name   string
	in     string
	stream string
	plain  string
}

func keptColourCases() []colourCase {
	npmLine := "\x1b[0m\x1b[37;40mnpm\x1b[0m \x1b[0m\x1b[30;43mWARN\x1b[0m \x1b[0m\x1b[35mdeprecated\x1b[0m pkg@1.0.0: moved"
	longestSGR := "\x1b[" + strings.Repeat("1", 32) + "mx\x1b[0m"

	cases := []colourCase{
		{in: "\x1b[31mred\x1b[0m", plain: "red"},
		{in: "\x1b[38;5;208morange\x1b[0m", plain: "orange"},
		{in: "\x1b[38:2::255:0:0mx\x1b[0m", plain: "x"},
		{in: "a\x1b[mb", plain: "ab"},
		{in: "\x1b[1mbold\x1b[0m plain", plain: "bold plain"},
		{in: longestSGR, plain: "x"},
		{in: npmLine, plain: "npm WARN deprecated pkg@1.0.0: moved"},
		{in: "\x1b[31m\x1b[0m", plain: ""},
		{in: "\x1b[31mred\x1b[m", plain: "red"},
		{in: "\x1b[31mred\x1b[00m", plain: "red"},
		{in: "\x1b[1;31mred\x1b[0;0m", plain: "red"},
		{in: "\x1b[31mred\x1b[;m", plain: "red"},
		{in: "\x1b[31mred\x1b[0m\nnext", plain: "red\nnext"},
		{
			in:     "a\x1b]0;t\x1b[31mred\x1b[0m",
			stream: "a\x1b[31mred\x1b[0m",
			plain:  "ared",
		},
		{
			name:   "8-bit CSI colour is kept in 7-bit form",
			in:     "\u009b31mred\u009b0m",
			stream: "\x1b[31mred\x1b[0m",
			plain:  "red",
		},
	}
	for i := range cases {
		if cases[i].stream == "" {
			cases[i].stream = cases[i].in
		}
		if cases[i].name == "" {
			cases[i].name = fmt.Sprintf("%q", cases[i].in)
		}
	}
	return cases
}

var closingCases = []sanitizeCase{
	{"red left on", "\x1b[31mred", "\x1b[31mred\x1b[0m"},
	{"default foreground is not a full reset", "\x1b[32mok\x1b[39m", "\x1b[32mok\x1b[39m\x1b[0m"},
	{"reset before a style", "\x1b[0;1mx", "\x1b[0;1mx\x1b[0m"},
	{"style before a reset", "\x1b[1;0mx", "\x1b[1;0mx"},
	{"reset after the last style", "\x1b[31mx\x1b[1m\x1b[0m", "\x1b[31mx\x1b[1m\x1b[0m"},
	{"style after the last reset", "\x1b[0m\x1b[31mx", "\x1b[0m\x1b[31mx\x1b[0m"},
	{"colon form is never a reset", "\x1b[0:0mx", "\x1b[0:0mx\x1b[0m"},
	{"256-colour black foreground", "\x1b[38;5;0mx", "\x1b[38;5;0mx\x1b[0m"},
	{"true-colour black background", "\x1b[48;2;0;0;0mx", "\x1b[48;2;0;0;0mx\x1b[0m"},
	{"true-colour red foreground", "\x1b[38;2;255;0;0mx", "\x1b[38;2;255;0;0mx\x1b[0m"},
	{"256-colour underline colour", "\x1b[58;5;0mx", "\x1b[58;5;0mx\x1b[0m"},
	{"256-colour with a missing index", "\x1b[38;5;mx", "\x1b[38;5;mx\x1b[0m"},
	{"256-colour after another style", "\x1b[1;38;5;0mx", "\x1b[1;38;5;0mx\x1b[0m"},
	{"256-colour after a reset", "\x1b[0;38;5;0mx", "\x1b[0;38;5;0mx\x1b[0m"},
	{"extended colour without operands", "\x1b[38mx", "\x1b[38mx\x1b[0m"},
	{"256-colour without an index", "\x1b[48;5mx", "\x1b[48;5mx\x1b[0m"},
	{"true-colour with missing operands", "\x1b[38;2;1;2mx", "\x1b[38;2;1;2mx\x1b[0m"},
	{"operands written with leading zeros", "\x1b[038;05;0mx", "\x1b[038;05;0mx\x1b[0m"},
	{"style after a 256-colour operand", "\x1b[38;5;0;1mx", "\x1b[38;5;0;1mx\x1b[0m"},
	{"reset after a 256-colour operand", "\x1b[38;5;0;0mx", "\x1b[38;5;0;0mx"},
	{"reset after true-colour operands", "\x1b[48;2;0;0;0;0mx", "\x1b[48;2;0;0;0;0mx"},
	{"reset after a style", "\x1b[31;0mx", "\x1b[31;0mx"},
	{"colour closed on a later line", "\x1b[31mred\nstill red", "\x1b[31mred\nstill red\x1b[0m"},
	{"no colour, nothing added", "plain", "plain"},
	{"a dropped sequence adds nothing", "x\x1b[2Jy", "xy"},
}

func TestSanitizeDropsControlSequences(t *testing.T) {
	for _, s := range sanitizers {
		for _, tc := range controlSequenceCases() {
			t.Run(s.name+"/"+tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, s.fn(tc.in))
			})
		}
	}
}

func TestSanitizeKeepsOrdinaryText(t *testing.T) {
	for _, s := range sanitizers {
		for _, text := range ordinaryTexts {
			t.Run(fmt.Sprintf("%s/%q", s.name, text), func(t *testing.T) {
				assert.Equal(t, text, s.fn(text))
			})
		}
	}
}

func TestSanitizeStreamKeepsColour(t *testing.T) {
	for _, tc := range keptColourCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.stream, SanitizeStream(tc.in))
			assert.Equal(t, tc.plain, Sanitize(tc.in))
		})
	}
}

func TestSanitizeRemovesAllColour(t *testing.T) {
	assert.Equal(t, "red", Sanitize("\x1b[31mred\x1b[0m"))

	for _, tc := range keptColourCases() {
		assert.NotContains(t, Sanitize(tc.in), "\x1b", tc.name)
	}
	for _, tc := range closingCases {
		assert.NotContains(t, Sanitize(tc.in), "\x1b", tc.name)
	}
}

func TestSanitizeStreamClosesOpenColour(t *testing.T) {
	for _, tc := range closingCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SanitizeStream(tc.in))
		})
	}
}

func TestSanitizeSplitSequenceLeavesNoEscape(t *testing.T) {
	frames := []struct{ in, want string }{
		{"abc\x1b[3", "abc"},
		{"1mdef", "1mdef"},
		{"abc\x1b", "abc"},
		{"abc\x1b]0;ti", "abc"},
		{"abc\x1b(", "abc"},
	}
	for _, s := range sanitizers {
		for _, frame := range frames {
			got := s.fn(frame.in)
			assert.Equal(t, frame.want, got, "%s(%q)", s.name, frame.in)
			assert.NotContains(t, got, "\x1b")
		}
	}
}

// seedInputs is every input of the tests above, for the fuzz target.
func seedInputs() []string {
	var seeds []string
	for _, tc := range controlSequenceCases() {
		seeds = append(seeds, tc.in)
	}
	seeds = append(seeds, ordinaryTexts...)
	for _, tc := range keptColourCases() {
		seeds = append(seeds, tc.in)
	}
	for _, tc := range closingCases {
		seeds = append(seeds, tc.in)
	}
	return append(seeds, "abc\x1b[3", "1mdef", "abc\x1b", "abc\x1b]0;ti", "abc\x1b(")
}

var (
	startsWithColour = regexp.MustCompile(`^\x1b\[[0-9;:]{0,32}m`)
	colourSequence   = regexp.MustCompile(`\x1b\[[0-9;:]{0,32}m`)
)

// randomUnits are the pieces random fields are made of. Escape introducers, parameters and final bytes
// are listed more than once so complete sequences turn up often.
var randomUnits = []string{
	"\x1b", "\x1b", "\x1b", "[", "[", "[", "]", `\`, "P", ";", ";", ":", "m", "m", "m", "?", ">",
	"0", "0", "1", "1", "2", "3", "7", "9", "a", "Z", " ",
	"\a", "\r", "\n", "\t", "\x00", "\x7f",
	"\x9b", "\x9d", "\x9c", "\xff", "\xc3",
	"\u009b", "\u202E", "\u00A0", "日", "\U0001F1EF\U0001F1F5",
}

func randomField(rng *rand.Rand) string {
	var b strings.Builder
	for range rng.Intn(41) {
		b.WriteString(randomUnits[rng.Intn(len(randomUnits))])
	}
	return b.String()
}

// outputFault says what is wrong with out, the result of sanitising a field, or returns "".
func outputFault(out string) string {
	if !utf8.ValidString(out) {
		return "not valid UTF-8"
	}
	for i := 0; i < len(out); i++ {
		switch c := out[i]; {
		case c == 0x1b:
			if !startsWithColour.MatchString(out[i:]) {
				return fmt.Sprintf("byte %d is an ESC that does not start a complete colour sequence", i)
			}
		case c == 0x7f || (c < 0x20 && c != '\t' && c != '\n'):
			return fmt.Sprintf("byte %d is the control byte %#02x", i, c)
		}
	}
	for _, r := range out {
		if r >= 0x80 && r <= 0x9f {
			return fmt.Sprintf("contains the C1 control %U", r)
		}
	}
	return ""
}

func checkSanitizeInvariants(tb testing.TB, in string) {
	tb.Helper()
	plain, stream := Sanitize(in), SanitizeStream(in)

	if strings.Contains(plain, "\x1b") {
		tb.Fatalf("Sanitize(%q) = %q keeps an ESC", in, plain)
	}
	if fault := outputFault(plain); fault != "" {
		tb.Fatalf("Sanitize(%q) = %q: %s", in, plain, fault)
	}
	if fault := outputFault(stream); fault != "" {
		tb.Fatalf("SanitizeStream(%q) = %q: %s", in, stream, fault)
	}
	if again := Sanitize(plain); again != plain {
		tb.Fatalf("Sanitize is not idempotent on %q: %q then %q", in, plain, again)
	}
	if again := SanitizeStream(stream); again != stream {
		tb.Fatalf("SanitizeStream is not idempotent on %q: %q then %q", in, stream, again)
	}
	if stripped := colourSequence.ReplaceAllString(stream, ""); stripped != plain {
		tb.Fatalf("SanitizeStream(%q) = %q differs from Sanitize(%q) = %q beyond colour", in, stream, in, plain)
	}
}

func TestSanitizeInvariantOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	withColour := 0
	for range 20000 {
		in := randomField(rng)
		checkSanitizeInvariants(t, in)
		if strings.Contains(SanitizeStream(in), "\x1b") {
			withColour++
		}
	}

	assert.Greater(t, withColour, 200, "the random fields should often contain a kept colour sequence")
}

func FuzzSanitize(f *testing.F) {
	for _, seed := range seedInputs() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkSanitizeInvariants(t, in)
	})
}
