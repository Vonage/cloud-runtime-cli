package root

import (
	"context"
	"errors"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"vcr-cli/pkg/api"
	"vcr-cli/testutil"
	"vcr-cli/testutil/mocks"
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

			f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil)

			output, err := checkForUpdate(context.Background(), f, tt.mock.RootCurrentVersion)
			if err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}

			require.Equal(t, tt.want.output, output)
		})
	}
}
