package configure_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

var errBoom = errors.New("boom")

func TestConfigureErrorsAreWrapped(t *testing.T) {
	good := stubPrompter{input: account, password: secret, selection: euw1Label}
	tests := []struct {
		name      string
		prompter  stubPrompter
		overrides []fakeplatform.Override
		wantStart string
	}{
		{"the key prompt fails", withInputErr(good, errBoom), nil, "failed to read api key: boom"},
		{"the secret prompt fails", withPasswordErr(good, errBoom), nil, "failed to read api secret: boom"},
		{
			"the region list fails", good,
			[]fakeplatform.Override{{Match: "GET /regions", Status: 500, Body: `{"error":{"code":1001,"message":"boom"}}`}},
			"failed to retrieve regions: ",
		},
		{"the region prompt fails", withSelectionErr(good, errBoom), nil, "failed to select region: boom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := start(t, setup{srv: startFake(t, tc.overrides...), prompter: tc.prompter})

			err := r.execute(t)

			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), tc.wantStart), "got %q", err.Error())
			assert.NoFileExists(t, r.path, "nothing is written before the region is chosen")
			assert.Empty(t, r.Stdout.String())
		})
	}
}

func TestConfigureKeepsTheFileWhenARegionCannotBeChosen(t *testing.T) {
	stored := config.File{GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.use1", APIKey: "old-key", APISecret: "old-secret"}
	good := stubPrompter{input: account, password: secret, selection: euw1Label}
	r := start(t, setup{stored: &stored, prompter: withSelectionErr(good, errBoom)})

	err := r.execute(t)

	require.EqualError(t, err, "failed to select region: boom")
	assertStored(t, r.path, stored)
}

func TestConfigureServerErrorKeepsItsDetails(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /regions", Status: 500, Body: `{"error":{"code":1001,"message":"boom"}}`})
	r := start(t, setup{srv: srv, script: answers()})

	err := r.execute(t)

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 500, apiErr.HTTPStatus)
	assert.Equal(t, "boom", apiErr.Message)
}

func TestConfigureReportsAConfigFileThatCannotBeWritten(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("a file, not a directory"), 0o600))
	path := filepath.Join(blocker, ".vcr-cli")
	r := start(t, setup{flags: config.Flags{ConfigFile: path}, script: answers()})

	err := r.execute(t)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to write config: "), "got %q", err.Error())
	assert.Empty(t, r.Stdout.String(), "no success line when the file was not written")
}

func TestConfigureRegionListEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /regions", Hang: true})
	r := start(t, setup{srv: srv, flags: config.Flags{Timeout: 200 * time.Millisecond}, script: answers()})

	err := runPastTheDeadline(t, r)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to retrieve regions: "), "got %q", err.Error())
	assert.NoFileExists(t, r.path)
}

func withInputErr(p stubPrompter, err error) stubPrompter {
	p.inputErr = err
	return p
}

func withPasswordErr(p stubPrompter, err error) stubPrompter {
	p.passwordErr = err
	return p
}

func withSelectionErr(p stubPrompter, err error) stubPrompter {
	p.selectionErr = err
	return p
}
