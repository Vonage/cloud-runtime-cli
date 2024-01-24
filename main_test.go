package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

func Test_printError(t *testing.T) {
	cmd := &cobra.Command{}

	type mock struct {
		err           error
		cmd           *cobra.Command
		latestVersion string
	}
	type want struct {
		stdout string
		stderr string
	}
	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "generic error",
			mock: mock{
				err:           errors.New("the app crashed"),
				cmd:           cmd,
				latestVersion: "1.0.1",
			},
			want: want{
				stdout: "\n\nA new release of vcr is available: 0.0.1 → 1.0.1\nTo upgrade, run: vcr upgrade\n",
				stderr: "X the app crashed\n",
			},
		},
		{
			name: "Api error",
			mock: mock{
				err: fmt.Errorf("http error: %w", api.Error{
					HTTPStatusCode: 404,
					ServerCode:     3001,
					Message:        "Not Found",
					TraceId:        "1234",
					ContainerLogs:  "container logs",
				}),
				cmd:           cmd,
				latestVersion: "1.0.1",
			},
			want: want{
				stdout: "\n\nA new release of vcr is available: 0.0.1 → 1.0.1\nTo upgrade, run: vcr upgrade\n",
				stderr: "X API Error Encountered:\n \tMain issue : http error\n \tHTTP status : 404\n \tError code : 3001\n \tDetailed message : Not Found\n \tTrace ID : 1234\n \tContainer logs : container logs\n\n",
			},
		},
		{
			name: "Cobra flag error",
			mock: mock{
				err:           cmdutil.FlagErrorf("unknown flag --foo"),
				cmd:           cmd,
				latestVersion: "1.0.1",
			},
			want: want{
				stderr: "unknown flag --foo\nUsage:\n\n",
			},
		},
		{
			name: "unknown Cobra command error",
			mock: mock{
				err:           errors.New("unknown command foo"),
				cmd:           cmd,
				latestVersion: "1.0.1",
			},
			want: want{
				stderr: "unknown command foo\nUsage:\n\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			mockUpdateMessageChan := make(chan string)
			go func() {
				mockUpdateMessageChan <- tt.mock.latestVersion
			}()

			printError(ios, tt.mock.err, tt.mock.cmd, mockUpdateMessageChan)

			if tt.want.stdout != "" {
				require.Equal(t, tt.want.stdout, stdout.String())

			}
			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, stderr.String())

			}
		})
	}
}
