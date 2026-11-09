package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

func TestRunRequestBodies(t *testing.T) {
	h := newHarness(t)
	env := []manifest.Env{{Name: "A", Value: "1"}, {Name: "B", Secret: "MY_SECRET"}}
	security := &manifest.Security{Access: "authenticated", AuthMethod: "vonage_basic"}
	h.plan.Capabilities = []string{"messages-v1", "voice"}
	h.plan.Manifest = &manifest.Manifest{Instance: manifest.Instance{
		Environment:     env,
		Entrypoint:      []string{"node", "index.js"},
		Domains:         []string{"api.example.test"},
		BuildScript:     "./build.sh",
		Scaling:         manifest.Scaling{MinScale: 1, MaxScale: 3},
		Security:        security,
		HealthCheckPath: "/custom/health",
	}}

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, platform.ValidateRequest{
		ProjectID:        "p-1",
		APIApplicationID: testAppID,
		InstanceName:     "dev",
		Region:           "aws.euw1",
		Environment:      env,
		MinScale:         1,
		MaxScale:         3,
	}, h.api.validateReq)
	assert.Equal(t, platform.PackageRequest{
		SourceCodeKey:   "source-key-1",
		Entrypoint:      []string{"node", "index.js"},
		Capabilities:    manifest.Capabilities{Messages: "v1", Voice: "v0"},
		BuildScriptPath: "./build.sh",
		Runtime:         "nodejs18",
	}, h.api.packageReq)
	assert.Equal(t, platform.DeployRequest{
		PackageID:           "pkg-1",
		ProjectID:           "p-1",
		APIApplicationID:    testAppID,
		InstanceName:        "dev",
		Region:              "aws.euw1",
		Environment:         env,
		Domains:             []string{"api.example.test"},
		MinScale:            1,
		MaxScale:            3,
		Security:            security,
		HealthCheckEndpoint: "/custom/health",
	}, h.api.deployReq)
	assert.Same(t, security, h.api.deployReq.Security, "the security pointer is passed through, not copied")
}

func TestRunTakesTheSixValuesFromThePlanNotTheManifest(t *testing.T) {
	h := newHarness(t)
	h.plan.Manifest = &manifest.Manifest{
		Project: manifest.Project{Name: "manifest-project"},
		Instance: manifest.Instance{
			Name: "manifest-instance", Runtime: "python3", Region: "aws.use1", ApplicationID: "manifest-app",
			Capabilities: []string{"rtc"},
		},
	}

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "my-project", h.api.projectAsked)
	assert.Equal(t, "dev", h.api.validateReq.InstanceName)
	assert.Equal(t, "aws.euw1", h.api.validateReq.Region)
	assert.Equal(t, testAppID, h.api.validateReq.APIApplicationID)
	assert.Equal(t, "nodejs18", h.api.packageReq.Runtime)
	assert.Equal(t, manifest.Capabilities{}, h.api.packageReq.Capabilities)
}

func TestRunKeepsEmptyManifestListsNil(t *testing.T) {
	h := newHarness(t)
	h.plan.Manifest = &manifest.Manifest{}

	err := h.run()

	require.NoError(t, err)
	assert.Nil(t, h.api.validateReq.Environment)
	assert.Nil(t, h.api.packageReq.Entrypoint)
	assert.Nil(t, h.api.deployReq.Environment)
	assert.Nil(t, h.api.deployReq.Domains)
	assert.Nil(t, h.api.deployReq.Security)
	assert.Empty(t, h.api.deployReq.HealthCheckEndpoint)
	assert.Zero(t, h.api.deployReq.MinScale)
	assert.Zero(t, h.api.deployReq.MaxScale)
}

func TestRunWithoutAManifestUsesEmptyOnes(t *testing.T) {
	h := newHarness(t)
	h.plan.Manifest = nil

	err := h.run()

	require.NoError(t, err)
	assert.Nil(t, h.api.packageReq.Entrypoint)
}

func TestRunUsesTheSourceKeyThatTheUploadReturned(t *testing.T) {
	h := newHarness(t)
	h.api.sourceKey = ""

	err := h.run()

	require.NoError(t, err)
	assert.Empty(t, h.api.packageReq.SourceCodeKey, "an empty key is the platform's to refuse, as before")
}
