package selfupdate

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgradeRefusesOversizedDownloads(t *testing.T) {
	t.Run("checksums.txt over 64 KiB", func(t *testing.T) {
		for name, handler := range map[string]http.HandlerFunc{
			"announced": declaring(64<<10+1, bytes.Repeat([]byte("x"), 64<<10+1)),
			"chunked":   chunked(64<<10 + 1),
		} {
			site := hostile{onList: handler}.start(t)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, frame(testTag, "checksums.txt is too large"), name)
			assert.Equal(t, before, snapshot(t, dir), name)
			assert.Len(t, site.seen(), 1, "%s: nothing is asked for after the list", name)
		}
	})
	t.Run("a signature over 4 KiB", func(t *testing.T) {
		for name, handler := range map[string]http.HandlerFunc{
			"announced": declaring(4<<10+1, bytes.Repeat([]byte("x"), 4<<10+1)),
			"chunked":   chunked(4<<10 + 1),
		} {
			site := hostile{onSignature: handler}.start(t)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, frame(testTag, signatureAsset(t)+" is too large"), name)
			assert.Equal(t, before, snapshot(t, dir), name)
			assert.Len(t, site.seen(), 2, "%s: the archive is not asked for", name)
		}
	})
	t.Run("an archive whose announced length is over the limit is not streamed", func(t *testing.T) {
		lowerLimit(t, &maxArchiveBytes, 1<<10)
		wrote := make(chan int, 1)
		site := hostile{onArchive: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(1<<10+1))
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			wrote <- 0 // the handler never wrote a body byte: it only waited for the client to go away
		}}.start(t)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		start := time.Now()
		err := site.client.Upgrade(t.Context(), site.release, target)

		require.EqualError(t, err, frame(testTag, thisAsset+" is too large"))
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.Equal(t, before, snapshot(t, dir))
		assert.Equal(t, 0, <-wrote)
	})
	t.Run("an archive that grows past the limit", func(t *testing.T) {
		lowerLimit(t, &maxArchiveBytes, 1<<10)
		site := hostile{onArchive: chunked(1 << 20)}.start(t)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := site.client.Upgrade(t.Context(), site.release, target)

		require.EqualError(t, err, frame(testTag, thisAsset+" is too large"))
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("an entry over the limit", func(t *testing.T) {
		lowerLimit(t, &maxEntryBytes, 64)
		site := hostile{archive: archiveOf(t, regularFile("vcr", bytes.Repeat([]byte("x"), 65)))}.start(t)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := site.client.Upgrade(t.Context(), site.release, target)

		require.EqualError(t, err, frame(testTag, reasonBadArchive))
		assert.Equal(t, before, snapshot(t, dir), "no temporary file is left")
	})
	t.Run("an entry exactly at the limit is installed", func(t *testing.T) {
		lowerLimit(t, &maxEntryBytes, 64)
		body := bytes.Repeat([]byte("x"), 64)
		site := hostile{archive: archiveOf(t, regularFile("vcr", body))}.start(t)
		_, target := installTarget(t)

		require.NoError(t, site.client.Upgrade(t.Context(), site.release, target))

		assert.Equal(t, string(body), readFile(t, target))
	})
}

func TestDownloadLimitsAreTheDocumentedOnes(t *testing.T) {
	assert.EqualValues(t, 64<<10, maxChecksumsBytes)
	assert.EqualValues(t, 4<<10, maxSignatureBytes)
	assert.EqualValues(t, 100<<20, maxArchiveBytes)
	assert.EqualValues(t, 200<<20, maxEntryBytes)
}

func TestDownloadsAcceptOnlyHTTPSOrLoopbackHTTP(t *testing.T) {
	const elsewhere = "http://203.0.113.7/object"
	refused := func(url string) string { return frame(testTag, "download URL must use https: "+url) }

	t.Run("with an https release URL a plain-http asset on loopback is refused before any request", func(t *testing.T) {
		site := hostile{releaseURL: "https://api.example.test/repos/o/r"}.start(t)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := site.client.Upgrade(t.Context(), site.release, target)

		require.EqualError(t, err, refused(site.server.URL+"/list"))
		assert.Empty(t, site.seen(), "the server saw no request")
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("with a loopback release URL, plain http to another host is refused for each asset", func(t *testing.T) {
		for _, name := range []string{listAsset, signatureAsset(t), thisAsset} {
			site := hostile{}.start(t)
			for i := range site.release.Assets {
				if site.release.Assets[i].Name == name {
					site.release.Assets[i].BrowserDownloadURL = elsewhere
				}
			}
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, refused(elsewhere), name)
			assert.Equal(t, before, snapshot(t, dir), name)
		}
	})
	t.Run("with a loopback release URL, a redirect to plain http elsewhere is refused for each asset", func(t *testing.T) {
		redirect := func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, elsewhere, http.StatusFound) }
		tests := map[string]hostile{
			listAsset:         {onList: redirect},
			signatureAsset(t): {onSignature: redirect},
			thisAsset:         {onArchive: redirect},
		}
		for name, h := range tests {
			site := h.start(t)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, refused(elsewhere), name)
			assert.Equal(t, before, snapshot(t, dir), name)
		}
	})
	t.Run("an asset address with a user name and password is refused for each asset and nothing is sent to it", func(t *testing.T) {
		const reason = "download URL must not contain a user name or password"
		servedAt := map[string]string{listAsset: "/list", signatureAsset(t): "/signature", thisAsset: "/archive"}
		for name, path := range servedAt {
			site := hostile{}.start(t)
			for i := range site.release.Assets {
				if site.release.Assets[i].Name == name {
					site.release.Assets[i].BrowserDownloadURL = strings.Replace(site.release.Assets[i].BrowserDownloadURL, "http://", "http://user:hunter2@", 1)
				}
			}
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, frame(testTag, reason), name)
			assert.Equal(t, before, snapshot(t, dir), name)
			for _, r := range site.seen() {
				assertNoCredentials(t, r.Header)
				assert.NotEqual(t, path, r.Path, "%s: the refused address was never requested", name)
			}
		}
	})
	t.Run("loopback http works when the release URL is loopback", func(t *testing.T) {
		site := hostile{}.start(t)
		_, target := installTarget(t)

		require.NoError(t, site.client.Upgrade(t.Context(), site.release, target))
	})
}
