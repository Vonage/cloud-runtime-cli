package upgrade

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestUpgrade(t *testing.T) {
	type mock struct {
		UpgradeGetLatestReleaseTimes     int
		UpgradeReturnRelease             api.Release
		UpgradeGetLatestReleaseReturnErr error

		UpgradeGetAssetTimes     int
		UpgradeURL               string
		UpgradeReturnBytes       []byte
		UpgradeGetAssetReturnErr error
		UpgradeVersion           string
		UpgradeBuildDate         string
		UpgradeCommit            string
	}
	type want struct {
		errMsg string
		stdout string
		stderr string
	}

	tests := []struct {
		name string
		cli  string
		mock mock
		want want
	}{
		{
			name: "happy-path-to-date",
			cli:  "",
			mock: mock{

				UpgradeGetLatestReleaseTimes:     1,
				UpgradeReturnRelease:             api.Release{TagName: "v0.0.1"},
				UpgradeGetLatestReleaseReturnErr: nil,

				UpgradeGetAssetTimes:     0,
				UpgradeURL:               "",
				UpgradeReturnBytes:       nil,
				UpgradeGetAssetReturnErr: nil,
				UpgradeVersion:           "0.0.1",
				UpgradeBuildDate:         "",
				UpgradeCommit:            "",
			},
			want: want{
				stdout: "✓ You are using the latest version of vcr-cli (0.0.1)\n",
			},
		},
		{
			name: "happy-path-newer-version",
			cli:  "",
			mock: mock{

				UpgradeGetLatestReleaseTimes:     1,
				UpgradeReturnRelease:             api.Release{TagName: "v0.0.1"},
				UpgradeGetLatestReleaseReturnErr: nil,

				UpgradeGetAssetTimes:     0,
				UpgradeURL:               "",
				UpgradeReturnBytes:       nil,
				UpgradeGetAssetReturnErr: nil,
				UpgradeVersion:           "0.0.5",
				UpgradeBuildDate:         "",
				UpgradeCommit:            "",
			},
			want: want{
				stdout: "✓ Current version (0.0.5) is newer than the latest version (0.0.1) !\n",
			},
		},

		{
			name: "latest-release-version-error",
			cli:  "",
			mock: mock{

				UpgradeGetLatestReleaseTimes:     1,
				UpgradeReturnRelease:             api.Release{TagName: "test"},
				UpgradeGetLatestReleaseReturnErr: nil,

				UpgradeGetAssetTimes:     0,
				UpgradeURL:               "",
				UpgradeReturnBytes:       nil,
				UpgradeGetAssetReturnErr: nil,
				UpgradeVersion:           "0.0.1",
				UpgradeBuildDate:         "",
				UpgradeCommit:            "",
			},
			want: want{
				errMsg: "failed to get latest version: invalid update found: No Major.Minor.Patch elements found",
			},
		},

		{
			name: "upgrade-api-error",
			cli:  "",
			mock: mock{

				UpgradeGetLatestReleaseTimes:     1,
				UpgradeReturnRelease:             api.Release{},
				UpgradeGetLatestReleaseReturnErr: errors.New("api error"),

				UpgradeGetAssetTimes:     0,
				UpgradeURL:               "",
				UpgradeReturnBytes:       nil,
				UpgradeGetAssetReturnErr: nil,
				UpgradeVersion:           "0.0.1",
				UpgradeBuildDate:         "",
				UpgradeCommit:            "",
			},
			want: want{
				errMsg: "failed to get assets: api error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			releaseMock := mocks.NewMockReleaseInterface(ctrl)
			releaseMock.EXPECT().
				GetLatestRelease(gomock.Any()).
				Times(tt.mock.UpgradeGetLatestReleaseTimes).
				Return(tt.mock.UpgradeReturnRelease, tt.mock.UpgradeGetLatestReleaseReturnErr)

			releaseMock.EXPECT().
				GetAsset(gomock.Any(), tt.mock.UpgradeURL).
				Times(tt.mock.UpgradeGetAssetTimes).
				Return(tt.mock.UpgradeReturnBytes, tt.mock.UpgradeGetAssetReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil)

			cmd := NewCmdUpgrade(f, tt.mock.UpgradeVersion, tt.mock.UpgradeBuildDate, tt.mock.UpgradeCommit)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			if _, err := cmd.ExecuteC(); err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}
			cmdOut := &testutil.CmdOut{
				OutBuf: stdout,
				ErrBuf: stderr,
			}
			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
				return
			}
			require.NoError(t, err, "should not throw error")
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}
}

func TestUpgradeByAsset(t *testing.T) {
	filePath := "testdata/vcr"

	file, err := os.Open(filePath)
	if err != nil {
		require.Error(t, err, "should throw open file error")
	}
	defer file.Close()

	byteSlice, err := io.ReadAll(file)
	if err != nil {
		require.Error(t, err, "should throw read file error")
	}

	type mock struct {
		UpgradeExePath           string
		UpgradeRelease           api.Release
		UpgradeGetAssetTimes     int
		UpgradeReturnAsset       []byte
		UpgradeGetAssetReturnErr error
	}
	type want struct {
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
				UpgradeExePath: "testdata/vcr",
				UpgradeRelease: api.Release{
					TagName: "v0.0.1",
					Assets:  []api.Asset{{Name: fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "test-DownloadURL"}}},
				UpgradeGetAssetTimes:     1,
				UpgradeReturnAsset:       byteSlice,
				UpgradeGetAssetReturnErr: nil,
			},

			want: want{
				errMsg: "",
			},
		},

		{
			name: "api-error",
			mock: mock{
				UpgradeExePath: "testdata/vcr",
				UpgradeRelease: api.Release{
					TagName: "v0.0.1",
					Assets:  []api.Asset{{Name: fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "test-DownloadURL"}}},
				UpgradeGetAssetTimes:     1,
				UpgradeReturnAsset:       nil,
				UpgradeGetAssetReturnErr: errors.New("api error"),
			},

			want: want{
				errMsg: "failed to get release asset: api error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			releaseMock := mocks.NewMockReleaseInterface(ctrl)
			releaseMock.EXPECT().
				GetAsset(gomock.Any(), gomock.Any()).
				Times(tt.mock.UpgradeGetAssetTimes).
				Return(tt.mock.UpgradeReturnAsset, tt.mock.UpgradeGetAssetReturnErr)

			f := testutil.DefaultFactoryMock(t, nil, nil, releaseMock, nil, nil, nil)
			opts := &Options{
				Factory: f,
			}
			err := updateByAsset(context.Background(), opts, tt.mock.UpgradeRelease, tt.mock.UpgradeExePath)
			if err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}

			require.NoError(t, err, "should not throw error")
		})
	}
}

func TestFormat(t *testing.T) {
	version := "1.0.0"
	buildDate := "2022-01-01"
	commit := "abcdefg"

	expectedOutput := fmt.Sprintf("vcr-cli version %s (commit:%s, date:%s)\n", version, commit, buildDate)

	output := Format(version, buildDate, commit)

	if output != expectedOutput {
		t.Errorf("Expected output: %q, but got: %q", expectedOutput, output)
	}
}
