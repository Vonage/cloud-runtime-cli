package initcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// shellSafe matches a path that a shell reads as one word without quoting.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// nextStep is the hint printed once the project is made: where to go and what to run there.
func nextStep(dir string) string {
	path := displayDir(dir)
	if path == "." {
		return "Next: vcr deploy"
	}
	return "Next: cd " + shellQuote(path) + " && vcr deploy"
}

// displayDir is how the hint names dir: relative to the working directory when dir is inside it ("." for the
// working directory itself), else the path whole.
func displayDir(dir string) string {
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dir
	}
	return rel
}

// shellQuote writes path as one shell word: bare when every character is safe, else in single quotes.
func shellQuote(path string) string {
	if shellSafe.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
