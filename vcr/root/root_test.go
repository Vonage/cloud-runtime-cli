package root

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestCheckForUpdate(t *testing.T) {
	type mock struct {
		RootCurrentVersion string

		RootGetLatestReleaseTimes       int
		RootReturnRelease               api.Release
		RootGetLatestReleaseReturnError error
	}
	type want struct {
		output string
		errMsg string
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			mock: mock{
				RootCurrentVersion: "0.0.1",

				RootGetLatestReleaseTimes:       1,
				RootReturnRelease:               api.Release{TagName: "v1.0.1"},
				RootGetLatestReleaseReturnError: nil,
			},
			want: want{
				output: "1.0.1",
				errMsg: "",
			},
		},
		{
			name: "api-error",
			mock: mock{
				RootCurrentVersion: "0.0.1",

				RootGetLatestReleaseTimes:       1,
				RootReturnRelease:               api.Release{},
				RootGetLatestReleaseReturnError: errors.New("api error"),
			},
			want: want{
				output: "",
				errMsg: "api error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			releaseMock := mocks.NewMockReleaseInterface(ctrl)

			releaseMock.EXPECT().GetLatestRelease(gomock.Any()).
				Times(tt.mock.RootGetLatestReleaseTimes).
				Return(tt.mock.RootReturnRelease, tt.mock.RootGetLatestReleaseReturnError)

			ios, _, _, _ := iostreams.Test()

			f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil, nil)

			output, err := checkForUpdate(t.Context(), f, tt.mock.RootCurrentVersion)
			if err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}

			require.Equal(t, tt.want.output, output)
		})
	}
}

// TestNewCmdRoot_registersTopLevelLogs pins the `vcr logs` shortcut to the root
// command's wiring. Asserting on Use (not just "a command was added") means
// registering NewCmdInstanceLog, whose Use is "log", fails here too.
func TestNewCmdRoot_registersTopLevelLogs(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)

	updateStream := make(chan string, 1)
	cmd := NewCmdRoot(f, "0.0.1", "2026-08-02", "abcdef0", updateStream)

	var uses []string
	found := false
	for _, sub := range cmd.Commands() {
		uses = append(uses, sub.Use)
		if sub.Use == "logs" {
			found = true
		}
	}
	require.True(t, found, `root must register a top-level command with Use == "logs"; got %v`, uses)
}
