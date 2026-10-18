package upgrade

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// versionShape is X.Y.Z with optional prerelease and build parts. semver.IsValid alone also accepts the shorthands
// "v1" and "v1.2".
var versionShape = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// textError is an error whose text is a whole sentence in the words of earlier releases, so it starts with a
// capital letter.
type textError string

func (e textError) Error() string { return string(e) }

// parseVersion returns v without a leading "v" when it is X.Y.Z with an optional prerelease and build part. The
// errors read as they did in earlier releases: a missing part, a part with a character that is not a digit, and
// anything else that is not a semantic version.
func parseVersion(v string) (string, error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return "", textError("No Major.Minor.Patch elements found")
	}
	patch := parts[2]
	if end := strings.IndexAny(patch, "+-"); end >= 0 {
		patch = patch[:end]
	}
	for _, number := range []struct{ name, text string }{{"major", parts[0]}, {"minor", parts[1]}, {"patch", patch}} {
		if !onlyDigits(number.text) {
			return "", textError(fmt.Sprintf("Invalid character(s) found in %s number %q", number.name, number.text))
		}
	}
	if !versionShape.MatchString(v) || !semver.IsValid("v"+v) {
		return "", textError("invalid semantic version")
	}
	return v, nil
}

func onlyDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// compareVersions orders two versions that parseVersion accepted: -1, 0 or +1. Build metadata does not count.
func compareVersions(a, b string) int { return semver.Compare("v"+a, "v"+b) }
