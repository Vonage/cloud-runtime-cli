package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionLine(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "release build prints its version unchanged",
			info: Info{
				Version: "v2.6.0",
				Commit:  "91034ccb3590a11a9db2077ac346e4bad84ff6f5",
				Date:    "2026-05-21T11:22:52Z",
			},
			want: "vcr-cli version v2.6.0 (commit:91034ccb3590a11a9db2077ac346e4bad84ff6f5, date:2026-05-21T11:22:52Z)",
		},
		{
			name: "unstamped development build prints 0.0.1",
			info: Info{Version: "dev", Commit: "0000", Date: "2021-09-01T00:00:00Z"},
			want: "vcr-cli version 0.0.1 (commit:0000, date:2021-09-01T00:00:00Z)",
		},
		{
			name: "release without a date leaves out the commit and date",
			info: Info{Version: "v2.6.0", Commit: "0000"},
			want: "vcr-cli version v2.6.0",
		},
		{
			name: "development build without a date leaves out the commit and date",
			info: Info{Version: "dev", Commit: "0000"},
			want: "vcr-cli version 0.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.VersionLine()

			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "\n")
		})
	}
}

func TestUserAgent(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: "v2.7.0", want: "vcr-cli/v2.7.0"},
		{version: "dev", want: "vcr-cli/dev"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.want, Info{Version: tt.version}.UserAgent())
		})
	}
}
