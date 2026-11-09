package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestBadDefaultRegionDoesNotBlockDeploy(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	settings := config.Settings{File: config.File{DefaultRegion: "aws.nope"}, Region: "aws.nope"}
	env, _, _ := newEnv(t, srv, settings, good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.NoError(t, err)
	for _, r := range srv.Requests() {
		assert.NotContains(t, r.Path, "aws.nope", "%s %s", r.Method, r.Path)
	}
}

func TestUnknownRegionFromTheFlagIsWrappedAsCli(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	settings := config.Settings{File: config.File{DefaultRegion: "aws.euw1"}, Region: "aws.nope"}
	env, _, _ := newEnv(t, srv, settings, good)
	env.Flags.Region = "aws.nope"
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.EqualError(t, err, "failed to initialize cli: region does not exist")
	assert.Equal(t, []string{"GET /regions/aws.nope"}, requestsOf(srv))
}

func TestUnknownDefaultRegionIsWrappedAsCli(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	settings := config.Settings{File: config.File{DefaultRegion: "aws.nope"}, Region: "aws.nope"}
	env, _, _ := newEnv(t, srv, settings, good)
	dir := writeProject(t, manifestYAML("my-project", "", ""))

	err := execute(t, env, dir)

	require.EqualError(t, err, "failed to initialize cli: region does not exist")
	assert.Equal(t, []string{"GET /regions/aws.nope"}, requestsOf(srv))
}

func TestNoRegionAnywhereIsWrappedAsCli(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, config.Settings{}, good)
	dir := writeProject(t, manifestYAML("my-project", "", ""))

	err := execute(t, env, dir)

	require.EqualError(t, err, "failed to initialize cli: region does not exist")
	assert.Equal(t, []string{"GET /regions"}, requestsOf(srv))
}

func TestUnknownManifestRegionIsWrappedAsDeploymentClient(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.nope", ""))

	err := execute(t, env, dir)

	require.EqualError(t, err, "failed to initialize deployment client: region does not exist")
	assert.Equal(t, []string{"GET /regions/aws.nope"}, requestsOf(srv))
}

func TestManifestRegionFailingForAnotherReasonIsWrappedAsCli(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	wrong := platform.Credentials{APIKey: testKey, APISecret: "wrong"}
	env, _, _ := newEnv(t, srv, defaultSettings(), wrong)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", ""))

	err := execute(t, env, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize cli: ")
	assert.NotContains(t, err.Error(), "deployment client")
	assert.Equal(t, []string{"GET /regions/aws.euw1"}, requestsOf(srv))
}

func TestCredentialFailureWithNoRegionKeepsThePlatformsError(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	wrong := platform.Credentials{APIKey: testKey, APISecret: "wrong"}
	env, _, _ := newEnv(t, srv, config.Settings{}, wrong)
	dir := writeProject(t, manifestYAML("my-project", "", ""))

	err := execute(t, env, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize cli: ")
	assert.NotContains(t, err.Error(), "region does not exist")
	assert.Equal(t, []string{"GET /regions"}, requestsOf(srv))
}

func TestPathArgumentErrorMakesNoRequest(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, _ := newEnv(t, srv, defaultSettings(), good)

	err := execute(t, env, "./missing")

	require.EqualError(t, err, `failed to get absolute path of "./missing": path is not existed`)
	assert.Empty(t, requestsOf(srv))
	assert.Empty(t, stdout.String())
}

func TestMissingManifestMakesNoRequest(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)

	err := execute(t, env, t.TempDir())

	require.EqualError(t, err, "failed to find template manifest file: manifest file not found")
	assert.Empty(t, requestsOf(srv))
}

func TestManifestParseErrorMakesNoRequest(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, "project: [unterminated\n")

	err := execute(t, env, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read manifest file: ")
	assert.Empty(t, requestsOf(srv))
}
