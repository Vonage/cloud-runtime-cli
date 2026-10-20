package root

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/config"
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

// executeRoot runs the root command with its subcommands' own work stubbed out,
// so only the root's PersistentPreRunE decides the outcome. It points the
// command at an empty config file, so a test never reads the developer's own.
func executeRoot(t *testing.T, f *mocks.MockFactory, args []string) (chan string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(cfgPath, nil, 0o600))

	updateStream := make(chan string)
	cmd := NewCmdRoot(f, "v1.0.0", "", "", updateStream)
	cmd.SetArgs(append(append([]string{}, args...), "--config-file", cfgPath))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	sub, _, err := cmd.Find(args[:1])
	require.NoError(t, err)
	sub.RunE = func(*cobra.Command, []string) error { return nil }
	sub.Run = nil

	return updateStream, cmd.Execute()
}

func TestRegionFlagValidation(t *testing.T) {
	tests := []struct {
		name   string
		region string
		valid  bool
	}{
		{name: "euw1", region: "aws.euw1", valid: true},
		{name: "apse1", region: "aws.apse1", valid: true},
		{name: "digits-only-name", region: "aws.123", valid: true},
		{name: "name-at-length-limit", region: "aws." + strings.Repeat("a", 63), valid: true},
		{name: "no-prefix", region: "x"},
		{name: "prefix-only", region: "aws."},
		{name: "prefix-without-dot", region: "aws"},
		{name: "unknown-prefix", region: "gcp.euw1"},
		{name: "uppercase-prefix", region: "AWS.euw1"},
		{name: "uppercase-name", region: "aws.EUW1"},
		{name: "hyphen", region: "aws.eu-w1"},
		{name: "extra-label", region: "aws.euw1.example.com"},
		{name: "path", region: "aws.euw1/x"},
		{name: "userinfo", region: "aws.euw1@example.com"},
		{name: "port", region: "aws.euw1:8443"},
		{name: "fragment", region: "aws.euw1#"},
		{name: "query", region: "aws.euw1?x=1"},
		{name: "trailing-newline", region: "aws.euw1\n"},
		{name: "leading-space", region: " aws.euw1"},
		{name: "non-ascii", region: "aws.eüw1"},
		{name: "name-over-length-limit", region: "aws." + strings.Repeat("a", 64)},
	}
	commands := []struct {
		name string
		args []string
		// reached sets the expectation for the branch the command takes once the region is accepted.
		reached func(f *mocks.MockFactory, region *string)
	}{
		{
			name: "configure",
			args: []string{"configure"},
			reached: func(f *mocks.MockFactory, region *string) {
				f.EXPECT().SetGlobalOptions(gomock.Any()).Do(func(opts *config.GlobalOptions) { *region = opts.Region })
			},
		},
		{
			name: "upgrade",
			args: []string{"upgrade"},
			reached: func(f *mocks.MockFactory, region *string) {
				f.EXPECT().InitUpgrade(gomock.Any()).Do(func(opts *config.GlobalOptions) { *region = opts.Region })
			},
		},
	}
	for _, cmdCase := range commands {
		for _, tt := range tests {
			t.Run(cmdCase.name+"/"+tt.name, func(t *testing.T) {
				ios, _, _, _ := iostreams.Test()
				f := mocks.NewMockFactory(gomock.NewController(t))
				f.EXPECT().IOStreams().Return(ios).AnyTimes()

				var seen string
				if tt.valid {
					cmdCase.reached(f, &seen)
				}

				updateStream, err := executeRoot(t, f, append(append([]string{}, cmdCase.args...), "--region", tt.region))

				if tt.valid {
					require.NoError(t, err)
					require.Equal(t, tt.region, seen)
					return
				}
				require.EqualError(t, err, fmt.Sprintf("invalid --region %q: expected aws.<name>", tt.region))
				select {
				case _, ok := <-updateStream:
					require.False(t, ok, "update stream should be closed")
				default:
					t.Fatal("update stream should be closed so the caller does not wait for an update check")
				}
			})
		}
	}
}

func TestRegionFlagOmittedIsNotValidated(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := mocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().IOStreams().Return(ios).AnyTimes()
	f.EXPECT().SetGlobalOptions(gomock.Any())

	_, err := executeRoot(t, f, []string{"configure"})

	require.NoError(t, err)
}

func TestRegionFlagRejectedBeforeAnyOtherCommandRuns(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	// No Init expectation: a call to it fails the test.
	f := mocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().IOStreams().Return(ios).AnyTimes()

	_, err := executeRoot(t, f, []string{"secret", "--region", "aws.euw1/x"})

	require.EqualError(t, err, `invalid --region "aws.euw1/x": expected aws.<name>`)
}
