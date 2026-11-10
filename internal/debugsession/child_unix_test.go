//go:build !windows

package debugsession

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChildHasOwnProcessGroup(t *testing.T) {
	r := startHelper(t, "sleep")
	waitForOutput(t, r.out, helperReady)

	group, err := syscall.Getpgid(r.child.pid)

	require.NoError(t, err)
	assert.Equal(t, r.child.pid, group)
	assert.NotEqual(t, syscall.Getpgrp(), group)
}

func TestStopKillsProcessIgnoringTerm(t *testing.T) {
	r := startHelper(t, "ignore-term")
	waitForOutput(t, r.out, helperReady)

	require.NoError(t, r.child.Stop())

	assert.Equal(t, exitInfo{State: "signal: killed", Stopped: true}, r.exits.waitFirst(t))
	waitUntilGone(t, r.child.pid)
}
