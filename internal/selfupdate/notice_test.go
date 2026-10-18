package selfupdate

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/term"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		tag     string
		want    string
		wantErr string
	}{
		{name: "a development build is older than any release", current: "dev", tag: "v2.7.0", want: "2.7.0"},
		{name: "the same version is not newer", current: "v2.6.0", tag: "v2.6.0", want: ""},
		{name: "an older release is not newer", current: "v2.7.0", tag: "v2.6.1", want: ""},
		{name: "the current version may lack the v", current: "2.6.0", tag: "v2.7.0", want: "2.7.0"},
		{name: "the tag may lack the v", current: "v2.6.0", tag: "2.7.0", want: "2.7.0"},
		{name: "a patch release is newer", current: "v2.6.0", tag: "v2.6.1", want: "2.6.1"},
		{name: "versions compare as numbers", current: "v2.9.0", tag: "v2.10.0", want: "2.10.0"},
		{name: "a prerelease is never offered", current: "v2.6.0", tag: "v2.7.0-rc.1", want: ""},
		{name: "a build suffix is never offered", current: "v2.6.0", tag: "v2.7.0+build", want: ""},
		{name: "a current prerelease is older than its release", current: "v2.7.0-rc.1", tag: "v2.7.0", want: "2.7.0"},
		{
			name: "a two-part tag is not a version", current: "v2.6.0", tag: "v2.7",
			wantErr: `invalid update found: "2.7" is not a valid version`,
		},
		{
			name: "a tag with a leading zero is not a version", current: "v2.6.0", tag: "v02.7.0",
			wantErr: `invalid update found: "02.7.0" is not a valid version`,
		},
		{
			name: "a word is not a tag", current: "v2.6.0", tag: "garbage",
			wantErr: `invalid update found: "garbage" is not a valid version`,
		},
		{
			name: "an empty tag is not a version", current: "v2.6.0", tag: "",
			wantErr: `invalid update found: "" is not a valid version`,
		},
		{
			name: "the current version must parse", current: "garbage", tag: "v2.7.0",
			wantErr: `current update is invalid: "garbage" is not a valid version`,
		},
		{
			name: "a two-part current version is not a version", current: "v2.6", tag: "v2.7.0",
			wantErr: `current update is invalid: "2.6" is not a valid version`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Newer(tt.current, Release{TagName: tt.tag})

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrintNotice(t *testing.T) {
	const want = "\n\nA new release of vcr is available: %s → 2.7.0\nTo upgrade, run: vcr upgrade\n"

	tests := []struct {
		name    string
		current string
		shown   string
	}{
		{"a release build", "v2.6.1", "2.6.1"},
		{"a build without the v", "2.6.1", "2.6.1"},
		{"a development build", "dev", "0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, stdout, stderr := term.Test("", false)

			PrintNotice(streams, tt.current, "2.7.0")

			assert.Equal(t, fmt.Sprintf(want, tt.shown), stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}
