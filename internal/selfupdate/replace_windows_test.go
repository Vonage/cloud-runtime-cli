//go:build windows

package selfupdate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceWindowsMovesTheRunningExecutableAside(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr.exe", "first", 0o755)

	require.NoError(t, replace(target, writing("second")))

	assert.Equal(t, "second", readFile(t, target))
	assert.Equal(t, "first", readFile(t, target+".old"), "the replaced executable is kept aside")
	assert.ElementsMatch(t, []string{"vcr.exe", "vcr.exe.old"}, names(t, dir))

	require.NoError(t, replace(target, writing("third")))

	assert.Equal(t, "third", readFile(t, target))
	assert.Equal(t, "second", readFile(t, target+".old"), "the stale file was removed before the next one was moved aside")
	assert.ElementsMatch(t, []string{"vcr.exe", "vcr.exe.old"}, names(t, dir))
}

func TestReplaceWindowsPutsTheOldExecutableBackWhenTheSwapFails(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr.exe", "first", 0o755)
	failTheLastRename(t)

	err := replace(target, writing("second"))

	require.ErrorIs(t, err, errInjected)
	assert.Equal(t, "first", readFile(t, target))
	assert.Equal(t, []string{"vcr.exe"}, names(t, dir))
}
