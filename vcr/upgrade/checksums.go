package upgrade

import (
	"crypto/sha256"
	"strings"
)

const (
	checksumsHeader = "# vcr "
	hashHexLength   = 2 * sha256.Size
	hashSeparator   = "  "
	asciiSpace      = ' '
	asciiDelete     = 0x7f
)

// parseChecksums reads checksums.txt for the release tag and returns the SHA-256 (lower-case hex) of each asset
// it lists. The first line is exactly "# vcr <tag>"; every other line is "<64 lower-case hex>", two spaces and the
// asset name. One newline may end the file. Any other line, or a name listed twice, fails the whole file.
func parseChecksums(list []byte, tag string) (map[string]string, error) {
	lines := strings.Split(strings.TrimSuffix(string(list), "\n"), "\n")
	if lines[0] != checksumsHeader+tag {
		return nil, errOtherRelease
	}
	entries := make(map[string]string, len(lines)-1)
	for _, line := range lines[1:] {
		hash, name, ok := parseChecksumLine(line)
		if _, listed := entries[name]; !ok || listed {
			return nil, errMalformed
		}
		entries[name] = hash
	}
	return entries, nil
}

func parseChecksumLine(line string) (hash, name string, ok bool) {
	if len(line) <= hashHexLength+len(hashSeparator) {
		return "", "", false
	}
	hash, rest := line[:hashHexLength], line[hashHexLength:]
	name, ok = strings.CutPrefix(rest, hashSeparator)
	if !ok || !isLowerHex(hash) || !isAssetName(name) {
		return "", "", false
	}
	return hash, name, true
}

func isLowerHex(s string) bool {
	for i := range len(s) {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// isAssetName accepts any name made of visible characters: no white space and no control characters, so a stray
// carriage return or trailing space is a malformed line rather than a name that never matches.
func isAssetName(s string) bool {
	for i := range len(s) {
		if c := s[i]; c <= asciiSpace || c == asciiDelete {
			return false
		}
	}
	return s != ""
}
