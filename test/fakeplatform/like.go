package fakeplatform

import (
	"regexp"
	"strings"
)

// likeMatch reports whether s matches the SQL ILIKE pattern: % stands for
// any run of characters, _ for one character, everything else for itself,
// without regard to case.
func likeMatch(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("(?is)^")
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
