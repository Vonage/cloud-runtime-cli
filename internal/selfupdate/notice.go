package selfupdate

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	devVersion      = "dev"
	devVersionShown = "0.0.1"
)

// versionPattern is X.Y.Z with optional prerelease and build parts. semver.IsValid alone also accepts the
// shorthands "v1" and "v1.2".
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// Newer returns the latest version without the "v" when it is a stable X.Y.Z newer than current ("dev" counts as
// 0.0.1), and "" otherwise.
func Newer(current string, latest Release) (string, error) {
	cur := shownVersion(current)
	if !validVersion(cur) {
		return "", fmt.Errorf("current update is invalid: %q is not a valid version", cur)
	}
	lat := strings.TrimPrefix(latest.TagName, "v")
	if !validVersion(lat) {
		return "", fmt.Errorf("invalid update found: %q is not a valid version", lat)
	}
	if semver.Prerelease("v"+lat) != "" || semver.Build("v"+lat) != "" {
		return "", nil
	}
	if semver.Compare("v"+cur, "v"+lat) < 0 {
		return lat, nil
	}
	return "", nil
}

// PrintNotice prints v2.6.1's notice on stdout: a blank line pair, "A new release of vcr is available: <cur> → <new>"
// (yellow and cyan) and "To upgrade, run: vcr upgrade".
func PrintNotice(t *term.IO, current, latest string) {
	if t.Mode() != term.Classic {
		t.Notice(noticeText(current, latest))
		return
	}
	style := t.Style()
	fmt.Fprintf(t.Out, "\n\n%s %s → %s\nTo upgrade, run: vcr upgrade\n",
		style.Yellow("A new release of vcr is available:"), style.Cyan(shownVersion(current)), style.Cyan(latest))
}

// shownVersion is v without a leading "v"; a development build reads as 0.0.1.
func shownVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	if v == devVersion {
		return devVersionShown
	}
	return v
}

func validVersion(v string) bool {
	return versionPattern.MatchString(v) && semver.IsValid("v"+v)
}
