package clierr_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
)

var errSentinel = errors.New("sentinel")

func TestAPIErrorText(t *testing.T) {
	tests := []struct {
		name string
		err  *clierr.APIError
		want string
	}{
		{
			name: "all fields",
			err: &clierr.APIError{
				HTTPStatus: 500, ServerCode: 7, Message: "boom", TraceID: "t-1", ContainerLogs: "line one",
			},
			want: "API Error Encountered: ( HTTP status: 500 Error code: 7 Detailed message: boom Trace ID: t-1 Container logs: line one )",
		},
		{
			name: "status only",
			err:  &clierr.APIError{HTTPStatus: 404},
			want: "API Error Encountered: ( HTTP status: 404 )",
		},
		{
			name: "message only",
			err:  &clierr.APIError{Message: "boom"},
			want: "API Error Encountered: ( Detailed message: boom )",
		},
		{
			name: "server code only",
			err:  &clierr.APIError{ServerCode: 7},
			want: "API Error Encountered: ( Error code: 7 )",
		},
		{
			name: "trace id only",
			err:  &clierr.APIError{TraceID: "t-1"},
			want: "API Error Encountered: ( Trace ID: t-1 )",
		},
		{
			name: "container logs only",
			err:  &clierr.APIError{ContainerLogs: "line one"},
			want: "API Error Encountered: ( Container logs: line one )",
		},
		{
			name: "zero value",
			err:  &clierr.APIError{},
			want: "API Error Encountered: ( )",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}

func TestUsagefFormatsItsArguments(t *testing.T) {
	err := clierr.Usagef("unknown flag %q (%d)", "--x", 3)

	var usageErr *clierr.UsageError
	require.ErrorAs(t, err, &usageErr)
	assert.Equal(t, `unknown flag "--x" (3)`, err.Error())
}

func TestUsageErrorUnwrap(t *testing.T) {
	err := clierr.Usagef("bad: %w", errSentinel)

	assert.ErrorIs(t, err, errSentinel)
	inner := errors.Unwrap(err)
	require.Error(t, inner)
	assert.Equal(t, inner.Error(), err.Error())
	assert.Equal(t, "bad: sentinel", err.Error())
}
