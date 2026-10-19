package term

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planVocabulary builds, from pieces, the patterns of the working vocabulary that a public document must not carry, so
// that this file does not carry it either.
func planVocabulary() map[string]*regexp.Regexp {
	return map[string]*regexp.Regexp{
		"a task id":   regexp.MustCompile(`\bT` + `\d{2}\b`),
		"a SEC id":    regexp.MustCompile(`\bSEC` + `-\d+`),
		"a row index": regexp.MustCompile(`(?i)\bro` + `w \d+`),
	}
}

func TestReadmeDocumentsTerminalOutput(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	readme := string(raw)

	require.Contains(t, readme, "\n## Terminal output\n", "the README has its own section on the terminal output")
	_, section, _ := strings.Cut(readme, "\n## Terminal output\n")
	section, _, _ = strings.Cut(section, "\n## ")

	for _, want := range []string{"VCR_UI=classic", "VCR_UI=static", "NO_COLOR", "CI"} {
		assert.Contains(t, section, want)
	}
	for name, pattern := range planVocabulary() {
		assert.Falsef(t, pattern.MatchString(readme), "the README names %s", name)
	}
}

func TestReadmeTerminalOutputFollowsUsage(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	readme := string(raw)

	usage := strings.Index(readme, "\n## Usage\n")
	terminal := strings.Index(readme, "\n## Terminal output\n")
	setup := strings.Index(readme, "\n## Development Setup\n")

	require.GreaterOrEqual(t, usage, 0)
	require.GreaterOrEqual(t, terminal, 0)
	require.GreaterOrEqual(t, setup, 0)
	assert.Less(t, usage, terminal, "the section comes after the usage section")
	assert.Less(t, terminal, setup, "and before the sections for contributors")
}
