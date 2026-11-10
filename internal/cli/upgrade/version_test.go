package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{in: "2.7.0", want: "2.7.0"},
		{in: "v2.7.0", want: "2.7.0"},
		{in: "2.7.0-rc.1", want: "2.7.0-rc.1"},
		{in: "v2.7.0-rc.1+build.5", want: "2.7.0-rc.1+build.5"},
		{in: "2.7.0+build", want: "2.7.0+build"},
		{in: "0.0.1", want: "0.0.1"},
		{in: "dev", wantErr: "No Major.Minor.Patch elements found"},
		{in: "", wantErr: "No Major.Minor.Patch elements found"},
		{in: "v", wantErr: "No Major.Minor.Patch elements found"},
		{in: "2.7", wantErr: "No Major.Minor.Patch elements found"},
		{in: "x.1.2", wantErr: `Invalid character(s) found in major number "x"`},
		{in: "1.y.2", wantErr: `Invalid character(s) found in minor number "y"`},
		{in: "1.2.z", wantErr: `Invalid character(s) found in patch number "z"`},
		{in: "1.2.3.4", wantErr: `Invalid character(s) found in patch number "3.4"`},
		{in: "1.2.3z-rc.1", wantErr: `Invalid character(s) found in patch number "3z"`},
		{in: "1.2.-rc.1", wantErr: "invalid semantic version"},
		{in: "01.2.3", wantErr: "invalid semantic version"},
		{in: "1.02.3", wantErr: "invalid semantic version"},
		{in: "1.2.03", wantErr: "invalid semantic version"},
		{in: "..", wantErr: "invalid semantic version"},
		{in: "1.2.3-", wantErr: "invalid semantic version"},
		{in: "1.2.3-rc..1", wantErr: "invalid semantic version"},
		{in: "1.2.3-01", wantErr: "invalid semantic version"},
		{in: "1.2.3+", wantErr: "invalid semantic version"},
		{in: " 1.2.3", wantErr: `Invalid character(s) found in major number " 1"`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseVersion(tt.in)

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

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2.7.0", "2.7.0", 0},
		{"2.7.0", "2.7.1", -1},
		{"2.7.1", "2.7.0", 1},
		{"2.10.0", "2.9.0", 1},
		{"3.0.0", "2.99.99", 1},
		{"2.7.0-rc.1", "2.7.0", -1},
		{"2.7.0", "2.7.0-rc.1", 1},
		{"2.7.0-rc.2", "2.7.0-rc.10", -1},
		{"2.7.0+one", "2.7.0+two", 0},
		{"0.0.1", "2.6.1", -1},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, compareVersions(tt.a, tt.b))
		})
	}
}
