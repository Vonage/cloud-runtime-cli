package configure_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
)

// basicAuth returns the user and password the request carried.
func basicAuth(t *testing.T, header http.Header) (string, string) {
	t.Helper()
	user, password, ok := (&http.Request{Header: header}).BasicAuth()
	require.True(t, ok, "the request carries no Basic credentials")
	return user, password
}

func TestConfigureSecretPromptKeepsTheCurrentSecret(t *testing.T) {
	stored := config.File{APIKey: account, APISecret: secret}
	tests := []struct {
		name   string
		stored *config.File
		flags  config.Flags
	}{
		{"the stored secret", &stored, config.Flags{}},
		{"the secret flag", nil, config.Flags{APISecret: secret}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{""}, Selects: []string{euw1Label}}
			r := start(t, setup{stored: tc.stored, flags: tc.flags, script: script})

			err := r.execute(t)

			require.NoError(t, err)
			assert.Equal(t, cmdtest.Ask{Kind: "password", Message: keepSecretPrompt, Required: false}, script.Asked[1])
			assertStored(t, r.path, config.File{
				GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
			})
		})
	}
}

func TestConfigureSecretFlagWinsOverTheStoredSecret(t *testing.T) {
	stored := config.File{APIKey: account, APISecret: "stored-secret"}
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{""}, Selects: []string{euw1Label}}
	r := start(t, setup{stored: &stored, flags: config.Flags{APISecret: secret}, script: script})

	err := r.execute(t)

	require.NoError(t, err)
	assertStored(t, r.path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureTypedSecretReplacesTheCurrentOne(t *testing.T) {
	stored := config.File{GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret}
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{"different"}}
	r := start(t, setup{stored: &stored, script: script})

	err := r.execute(t)

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	reqs := r.Requests()
	require.Len(t, reqs, 1)
	user, password := basicAuth(t, reqs[0].Header)
	assert.Equal(t, account, user)
	assert.Equal(t, "different", password)
	assertStored(t, r.path, stored)
}

func TestConfigureSecretPromptWithoutACurrentSecret(t *testing.T) {
	r := start(t, setup{script: answers()})

	require.NoError(t, r.execute(t))

	assert.Equal(t, cmdtest.Ask{Kind: "password", Message: newSecretPrompt, Required: true}, r.script.Asked[1])
}

func TestConfigureNeverPrintsTheSecret(t *testing.T) {
	stored := config.File{GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret}
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{""}, Selects: []string{euw1Label}}
	r := start(t, setup{stored: &stored, flags: config.Flags{APISecret: secret}, script: script})

	err := r.execute(t)

	require.NoError(t, err)
	assert.NotContains(t, r.Stdout.String(), secret)
	assert.NotContains(t, r.Stderr.String(), secret)
	for _, ask := range script.Asked {
		assert.NotContains(t, ask.Message, secret)
		assert.NotContains(t, ask.Default, secret)
		assert.NotContains(t, strings.Join(ask.Options, "\n"), secret)
	}
}

func TestConfigureNeverPrintsTheSecretOfAFailedRun(t *testing.T) {
	stored := config.File{GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret}
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{"typed-secret"}}
	r := start(t, setup{stored: &stored, script: script})

	err := r.execute(t)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "typed-secret")
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, r.Stdout.String(), secret)
	assert.NotContains(t, r.Stderr.String(), secret)
}

func TestConfigureUsesTheEnteredCredentials(t *testing.T) {
	stored := config.File{GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: "stored-key", APISecret: "stored-secret"}
	r := start(t, setup{stored: &stored, script: answers()})

	err := r.execute(t)

	require.NoError(t, err)
	reqs := r.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, account, reqs[0].User)
	assertStored(t, r.path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureWrongCredentialsWriteNothing(t *testing.T) {
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{"wrong"}}
	r := start(t, setup{script: script})

	err := r.execute(t)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to retrieve regions: "), "got %q", err.Error())
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	assert.Equal(t, "invalid credentials", apiErr.Message)
	assert.NotContains(t, err.Error(), "wrong")
	assert.NoFileExists(t, r.path)
	assert.Empty(t, r.Stdout.String())
}
