package upgrade

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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

			f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil, nil)

			cmd := NewCmdUpgrade(f, tt.mock.UpgradeVersion)
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

func TestUpgradeRefusesADevelopmentBuild(t *testing.T) {
	for _, version := range []string{"dev", "garbage", "1.2", "v1.2.x", ""} {
		for _, args := range []string{"", "--force"} {
			t.Run(fmt.Sprintf("%q %q", version, args), func(t *testing.T) {
				// no GetLatestRelease expectation: asking GitHub fails the test
				releaseMock := mocks.NewMockReleaseInterface(gomock.NewController(t))
				ios, _, _, stderr := iostreams.Test()
				f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil, nil)
				cmd := NewCmdUpgrade(f, version)
				argv, err := shlex.Split(args)
				require.NoError(t, err)
				cmd.SetArgs(argv)
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)

				_, err = cmd.ExecuteC()

				require.EqualError(t, err, "Cannot upgrade a development build")
				assert.Empty(t, stderr.String())
			})
		}
	}
}

// runUpgradeCommand runs `vcr upgrade --force --path dir` as version 2.5.0 against r, with keys trusted.
func runUpgradeCommand(t *testing.T, r *fakeRelease, dir string, keys []trustedKey) (string, error) {
	t.Helper()
	releaseMock := mocks.NewMockReleaseInterface(gomock.NewController(t))
	releaseMock.EXPECT().GetLatestRelease(gomock.Any()).Return(r.release(), nil)
	releaseMock.EXPECT().GetAsset(gomock.Any(), gomock.Any()).DoAndReturn(r.get).AnyTimes()
	ios, _, stdout, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil, nil)
	cmd := newCmdUpgrade(f, "2.5.0", keys)
	cmd.SetArgs([]string{"--force", "--path", dir})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err := cmd.ExecuteC()
	return stdout.String(), err
}

func TestUpgradeCommandInstallsAVerifiedRelease(t *testing.T) {
	priv := seededKey("release")
	dir, _ := installTarget(t)

	stdout, err := runUpgradeCommand(t, newFakeRelease(t, priv), dir, []trustedKey{trustedKeyOf(priv)})

	require.NoError(t, err)
	assert.Contains(t, stdout, "Successfully updated to version 9.9.9")
	assert.Equal(t, map[string]string{"vcr": fakeBinary(testTag)}, snapshot(t, dir))
}

func TestUpgradeCommandChangesNothingWithoutATrustedKey(t *testing.T) {
	priv := seededKey("release")
	dir, _ := installTarget(t)
	before := snapshot(t, dir)

	stdout, err := runUpgradeCommand(t, newFakeRelease(t, priv), dir, nil)

	require.EqualError(t, err, frame(testTag, reasonNoSignature))
	assert.NotContains(t, stdout, "Successfully updated")
	assert.Equal(t, before, snapshot(t, dir))
}

func TestUpgradeCommandUsesTheKeysOfTheBuild(t *testing.T) {
	priv := seededKey("release")
	previous := testPublicKey
	t.Cleanup(func() { testPublicKey = previous })
	dir, _ := installTarget(t)
	before := snapshot(t, dir)

	t.Run("a release signed by a key that is not compiled in is refused", func(t *testing.T) {
		SetTestKey("")
		_, err := runUpgradeCommand(t, newFakeRelease(t, priv), dir, trustedKeys())

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("the test key of a test build is trusted", func(t *testing.T) {
		SetTestKey(publicKeyLine(priv))
		_, err := runUpgradeCommand(t, newFakeRelease(t, priv), dir, trustedKeys())

		require.NoError(t, err)
		assert.Equal(t, map[string]string{"vcr": fakeBinary(testTag)}, snapshot(t, dir))
	})
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
