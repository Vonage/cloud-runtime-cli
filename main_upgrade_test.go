package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
	"vonage-cloud-runtime-cli/vcr/upgrade"
)

// A release whose files cannot be downloaded is reported as a release that cannot be verified, not as an API
// failure, once printError has rendered it.
func TestPrintErrorShowsTheVerifyMessageWhenAReleaseFileCannotBeDownloaded(t *testing.T) {
	archive := fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if archive != "vcr_linux_amd64.tar.gz" && archive != "vcr_darwin_arm64.tar.gz" {
		t.Skip("the signed fixture lists the archives of linux/amd64 and darwin/arm64 only")
	}
	fixtures := filepath.Join("vcr", "upgrade", "testdata", "signing")
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(fixtures, name))
		require.NoError(t, err)
		return data
	}
	publicKey, list, signature := read("trusted.pub"), read("checksums.txt"), read("good.sig")
	blob, err := base64.StdEncoding.DecodeString(strings.Fields(string(publicKey))[1])
	require.NoError(t, err)
	sum := sha256.Sum256(blob[len(blob)-32:]) // the raw key is the last string of the blob
	signatureAsset := "checksums.txt." + hex.EncodeToString(sum[:8]) + ".sig"

	t.Cleanup(func() { upgrade.SetTestKey("") })
	upgrade.SetTestKey(string(publicKey))

	release := api.Release{TagName: "v9.9.9", Assets: []api.Asset{
		{Name: archive, BrowserDownloadURL: "https://release.test/" + archive},
		{Name: "checksums.txt", BrowserDownloadURL: "https://release.test/checksums.txt"},
		{Name: signatureAsset, BrowserDownloadURL: "https://release.test/" + signatureAsset},
	}}
	bodies := map[string][]byte{"checksums.txt": list, signatureAsset: signature, archive: []byte("archive")}

	tests := []struct {
		fails  string
		status int
	}{
		{signatureAsset, 500},
		{"checksums.txt", 500},
		{"checksums.txt", 404},
		{archive, 500},
		{archive, 404},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s answers %d", tt.fails, tt.status), func(t *testing.T) {
			releaseMock := mocks.NewMockReleaseInterface(gomock.NewController(t))
			releaseMock.EXPECT().GetLatestRelease(gomock.Any()).Return(release, nil)
			releaseMock.EXPECT().GetAsset(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, url string) ([]byte, error) {
				name := strings.TrimPrefix(url, "https://release.test/")
				if name == tt.fails {
					return nil, api.Error{HTTPStatusCode: tt.status, Message: "server said no", TraceID: "1234"}
				}
				return bodies[name], nil
			}).AnyTimes()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vcr"), []byte("old"), 0o755))
			ios, _, _, stderr := iostreams.Test()
			f := testutil.DefaultFactoryMock(t, ios, nil, releaseMock, nil, nil, nil, nil)
			cmd := upgrade.NewCmdUpgrade(f, "2.5.0")
			cmd.SetArgs([]string{"--force", "--path", dir})

			ran, err := cmd.ExecuteC()
			require.Error(t, err)
			updates := make(chan string)
			close(updates)
			printError(ios, err, ran, updates)

			want := "Cannot verify vcr v9.9.9: could not download " + tt.fails +
				". Nothing was changed. Download it from https://github.com/Vonage/cloud-runtime-cli/releases and check it by hand."
			assert.Equal(t, "X "+want+"\n", stderr.String())
			content, err := os.ReadFile(filepath.Join(dir, "vcr"))
			require.NoError(t, err)
			assert.Equal(t, "old", string(content))
		})
	}
}
