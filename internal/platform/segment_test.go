package platform

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegEscapesOrdinaryValues(t *testing.T) {
	tests := map[string]string{
		"my-app":                   "my-app",
		"a b":                      "a%20b",
		"a?b":                      "a%3Fb",
		"a#b":                      "a%23b",
		"é":                        "%C3%A9",
		"a:b":                      "a:b",
		"a;b":                      "a%3Bb",
		"v0.3":                     "v0.3",
		"..a":                      "..a",
		"a..":                      "a..",
		strings.Repeat("x", 256):   strings.Repeat("x", 256),
		"aws.euw1":                 "aws.euw1",
		"3f2a9d84-0a54-4e3c-9a64":  "3f2a9d84-0a54-4e3c-9a64",
		"name with spaces & marks": "name%20with%20spaces%20&%20marks",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := seg(in)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestSegRejectsUnsafeValues(t *testing.T) {
	tests := map[string]string{
		"empty":            "",
		"dot":              ".",
		"double dot":       "..",
		"too long":         strings.Repeat("x", 257),
		"slash":            "a/b",
		"backslash":        `a\b`,
		"percent":          "a%2Fb",
		"nul":              "a\x00b",
		"escape":           "a\x1bb",
		"newline":          "a\nb",
		"delete":           "a\x7fb",
		"carriage return":  "a\rb",
		"tab":              "a\tb",
		"leading slash":    "/etc",
		"encoded dot dots": "%2e%2e",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := seg(in)

			require.Error(t, err)
			assert.Empty(t, got)
			assert.Contains(t, err.Error(), "invalid path value ")
		})
	}
}

func TestSegTruncatesTheValueInItsError(t *testing.T) {
	_, err := seg(strings.Repeat("é", 100) + "/")

	require.Error(t, err)
	assert.LessOrEqual(t, len(err.Error()), len(`invalid path value ""`)+40)
	assert.True(t, strings.HasPrefix(err.Error(), `invalid path value "é`))
}

func TestCheckServerID(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"3f2a9d84-0a54-4e3c-9a64-1c2d3e4f5a6b", true},
		{"abc_123.x-y", true},
		{strings.Repeat("a", 128), true},
		{"", false},
		{".", false},
		{"..", false},
		{"../x", false},
		{"a/b", false},
		{"a b", false},
		{"a%2f", false},
		{"a\nb", false},
		{"é", false},
		{strings.Repeat("a", 129), false},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			err := checkServerID("instance id", tc.value)

			if tc.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "the platform returned an invalid instance id: "), err.Error())
		})
	}
}

func TestCheckServerIDShowsASafeValue(t *testing.T) {
	err := checkServerID("package id", "bad\x1b[31m\x00"+strings.Repeat("é", 300))

	require.Error(t, err)
	text := err.Error()
	assert.NotContains(t, text, "\x1b")
	assert.NotContains(t, text, "\x00")
	assert.LessOrEqual(t, len(text), len("the platform returned an invalid package id: ")+200)
	assert.True(t, strings.HasPrefix(text, "the platform returned an invalid package id: bad[31m"), text)
}

func TestCheckServiceName(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"debug-abc123", true},
		{"A1", true},
		{"a-", true},
		{strings.Repeat("a", 63), true},
		{"-a", false},
		{"a.b", false},
		{strings.Repeat("a", 64), false},
		{"a_b", false},
		{"", false},
		{"../x", false},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			err := checkServiceName(tc.value)

			if tc.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "the platform returned an invalid service name: "), err.Error())
		})
	}
}

func TestEndpointBuildsURLFromSegments(t *testing.T) {
	base := mustURL(t, "https://api.example.test/gw/?ignored=1#frag")
	name, err := seg("a b")
	require.NoError(t, err)

	u := endpoint(base, "v0.3", "applications", name)
	u.RawQuery = url.Values{"filter": {"a b&c"}, "limit": {"3"}}.Encode()

	assert.Equal(t, "https://api.example.test/gw/v0.3/applications/a%20b?filter=a+b%26c&limit=3", u.String())
	assert.Equal(t, "/gw/v0.3/applications/a%20b", u.EscapedPath())
	assert.Equal(t, "https://api.example.test/gw/?ignored=1#frag", base.String(), "the base is not modified")
}

func TestEndpointKeepsEscapedSegmentsApart(t *testing.T) {
	semicolon, err := seg("a;b")
	require.NoError(t, err)
	question, err := seg("a?b")
	require.NoError(t, err)

	u := endpoint(mustURL(t, "https://api.example.test"), "projects", semicolon, "instances", question)

	assert.Equal(t, "https://api.example.test/projects/a%3Bb/instances/a%3Fb", u.String())
	assert.Equal(t, "/projects/a;b/instances/a?b", u.Path)
}

func TestEndpointWithoutSegmentsKeepsTheBasePath(t *testing.T) {
	assert.Equal(t, "https://api.example.test/gw/", endpoint(mustURL(t, "https://api.example.test/gw/")).String())
	assert.Equal(t, "https://api.example.test/gw/v0.3", endpoint(mustURL(t, "https://api.example.test/gw"), "v0.3").String())
}
