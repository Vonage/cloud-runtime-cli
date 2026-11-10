package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
)

func TestStampVariables(t *testing.T) {
	assert.Equal(t, "dev", version)
	assert.Equal(t, "0000", commit)
	assert.Equal(t, "2021-09-01T00:00:00Z", buildDate)
	assert.Equal(t, "v0.5", apiVersion)
	assert.Equal(t, "https://api.github.com/repos/Vonage/cloud-runtime-cli", releaseURL)
	assert.Empty(t, upgradeTestKey)

	origVersion, origCommit, origBuildDate := version, commit, buildDate
	origAPIVersion, origReleaseURL, origUpgradeTestKey := apiVersion, releaseURL, upgradeTestKey
	t.Cleanup(func() {
		version, commit, buildDate = origVersion, origCommit, origBuildDate
		apiVersion, releaseURL, upgradeTestKey = origAPIVersion, origReleaseURL, origUpgradeTestKey
	})

	version, commit, buildDate = "v9.9.9", "abc123", "2030-01-02T03:04:05Z"
	apiVersion, releaseURL, upgradeTestKey = "v9.9", "https://api.example.test/repos/o/r", "test-key"

	assert.Equal(t, buildinfo.Info{
		Version:        "v9.9.9",
		Commit:         "abc123",
		Date:           "2030-01-02T03:04:05Z",
		APIVersion:     "v9.9",
		ReleaseURL:     "https://api.example.test/repos/o/r",
		UpgradeTestKey: "test-key",
	}, buildInfo())
}
