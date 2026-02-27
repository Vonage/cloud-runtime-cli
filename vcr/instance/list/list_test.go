package list

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func runCommand(t *testing.T, datastoreMock cmdutil.DatastoreInterface, cli string) (*testutil.CmdOut, error) {
	t.Helper()

	ios, _, stdout, stderr := iostreams.Test()

	argv, err := shlex.Split(cli)
	if err != nil {
		t.Fatal(err)
	}

	f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

	cmd := NewCmdInstanceList(f)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if _, err := cmd.ExecuteC(); err != nil {
		return nil, err
	}

	return &testutil.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, nil
}

func TestInstanceList(t *testing.T) {
	type mock struct {
		ListTimes           int
		ListWantFilter      string
		ListReturnInstances []api.InstanceListItem
		ListReturnErr       error
	}
	type want struct {
		errMsg   string
		contains []string
	}

	tests := []struct {
		name string
		cli  string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			cli:  "",
			mock: mock{
				ListTimes:      1,
				ListWantFilter: "",
				ListReturnInstances: []api.InstanceListItem{
					{
						ID:               "11111111-1111-1111-1111-111111111111",
						APIApplicationID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						Name:             "dev",
						ServiceName:      "my-service",
					},
					{
						ID:               "22222222-2222-2222-2222-222222222222",
						APIApplicationID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						Name:             "prod",
						ServiceName:      "my-service-prod",
					},
				},
			},
			want: want{
				contains: []string{
					"INSTANCE ID",
					"API APPLICATION ID",
					"INSTANCE NAME",
					"SERVICE NAME",
					"11111111-1111-1111-1111-111111111111",
					"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					"dev",
					"my-service",
					"22222222-2222-2222-2222-222222222222",
					"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					"prod",
					"my-service-prod",
				},
			},
		},
		{
			name: "with-filter",
			cli:  "--filter=prod",
			mock: mock{
				ListTimes:      1,
				ListWantFilter: "prod",
				ListReturnInstances: []api.InstanceListItem{
					{
						ID:               "22222222-2222-2222-2222-222222222222",
						APIApplicationID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						Name:             "prod",
						ServiceName:      "my-service-prod",
					},
				},
			},
			want: want{
				contains: []string{
					"INSTANCE ID",
					"API APPLICATION ID",
					"INSTANCE NAME",
					"SERVICE NAME",
					"22222222-2222-2222-2222-222222222222",
					"my-service-prod",
				},
			},
		},
		{
			name: "no-items",
			cli:  "",
			mock: mock{
				ListTimes:           1,
				ListWantFilter:      "",
				ListReturnInstances: []api.InstanceListItem{},
			},
			want: want{
				contains: []string{
					"INSTANCE ID",
					"API APPLICATION ID",
					"INSTANCE NAME",
					"SERVICE NAME",
				},
			},
		},
		{
			name: "with-api-error",
			cli:  "",
			mock: mock{
				ListTimes:     1,
				ListReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to list instances: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
			datastoreMock.EXPECT().
				ListInstances(gomock.Any(), tt.mock.ListWantFilter).
				Times(tt.mock.ListTimes).
				Return(tt.mock.ListReturnInstances, tt.mock.ListReturnErr)

			cmdOut, err := runCommand(t, datastoreMock, tt.cli)
			if tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}
			require.NoError(t, err, "should not throw error")
			for _, s := range tt.want.contains {
				require.Contains(t, cmdOut.String(), s)
			}
		})
	}
}
