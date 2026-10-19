//go:build windows

package config

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sharingViolation(oldpath, newpath string) error {
	return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errSharingViolation}
}

func TestRenameWithRetrySucceedsAfterSharingViolations(t *testing.T) {
	t.Parallel()
	var calls int
	rename := func(oldpath, newpath string) error {
		calls++
		if calls <= renameRetries {
			return sharingViolation(oldpath, newpath)
		}
		return nil
	}

	require.NoError(t, renameWithRetry(rename, "a", "b"))

	assert.Equal(t, renameRetries+1, calls, "the last retry succeeds")
}

func TestRenameWithRetryGivesUpAfterThreeRetries(t *testing.T) {
	t.Parallel()
	var calls int
	rename := func(oldpath, newpath string) error {
		calls++
		return sharingViolation(oldpath, newpath)
	}

	err := renameWithRetry(rename, "a", "b")

	require.ErrorIs(t, err, errSharingViolation)
	assert.Equal(t, 4, calls, "one attempt and three retries")
}

func TestRenameWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()
	var calls int
	cause := errors.New("access denied")
	rename := func(string, string) error {
		calls++
		return cause
	}

	err := renameWithRetry(rename, "a", "b")

	assert.ErrorIs(t, err, cause)
	assert.Equal(t, 1, calls)
}
