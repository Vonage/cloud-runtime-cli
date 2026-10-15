package platform

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const uploadReply = `{"sourceCodeKey":"k1"}`

// writeArchive writes a gzip file holding size random bytes, so that it does not shrink, and returns its path and
// its content.
func writeArchive(t *testing.T, size int) (path string, content []byte) {
	t.Helper()
	payload := make([]byte, size)
	_, err := rand.Read(payload)
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err = zw.Write(payload)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	path = filepath.Join(t.TempDir(), "source.tar.gz")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return path, buf.Bytes()
}

// formPart is one part of a multipart body.
type formPart struct {
	Name        string
	FileName    string
	ContentType string
	Data        []byte
}

// boundaryOf returns the multipart boundary that the headers of a request declare.
func boundaryOf(t *testing.T, header http.Header) string {
	t.Helper()
	media, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", media)
	require.NotEmpty(t, params["boundary"])
	return params["boundary"]
}

// parseForm reads body the way a server does. It fails unless the closing delimiter is there: a reader that does
// not find it ends with an error and not with io.EOF.
func parseForm(t *testing.T, body []byte, boundary string) []formPart {
	t.Helper()
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var parts []formPart
	for {
		part, err := reader.NextPart()
		if err != nil {
			require.Equal(t, io.EOF, err, "the body must end with the closing delimiter")
			return parts
		}
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		parts = append(parts, formPart{
			Name: part.FormName(), FileName: part.FileName(), ContentType: part.Header.Get("Content-Type"), Data: data,
		})
	}
}

func TestUploadSourceWireFormat(t *testing.T) {
	path, archive := writeArchive(t, 64<<10)
	s := newRouteServer(t, answerJSON(http.StatusOK, uploadReply))

	key, err := s.client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	assert.Equal(t, "k1", key)
	req := s.only(t)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "/v0.3/packages/source", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assertCredentialsSent(t, req.Header)

	boundary := boundaryOf(t, req.Header)
	parts := parseForm(t, req.Body, boundary)
	require.Len(t, parts, 1)
	assert.Equal(t, formPart{Name: "tgz-code", FileName: "tgz-code.tar.gz", ContentType: "application/x-gzip", Data: archive}, parts[0])

	wantHead := "--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="tgz-code"; filename="tgz-code.tar.gz"` + "\r\n" +
		"Content-Type: application/x-gzip\r\n\r\n"
	assert.True(t, bytes.HasPrefix(req.Body, []byte(wantHead)), "the part headers are the ones v2.6.1 sent")
	assert.True(t, bytes.HasSuffix(req.Body, []byte("\r\n--"+boundary+"--\r\n")), "the closing delimiter")

	assert.Positive(t, req.ContentLength)
	assert.Equal(t, int64(len(req.Body)), req.ContentLength, "the declared length is the real one")
	assert.Empty(t, req.TransferEncoding, "the request is not chunked")
}

func TestUploadSourceDetectsTheContentTypeOfTheFile(t *testing.T) {
	tests := map[string]struct {
		content string
		want    string
	}{
		"text":  {"just some words\n", "text/plain; charset=utf-8"},
		"empty": {"", "text/plain; charset=utf-8"},
		"png":   {"\x89PNG\r\n\x1a\n and more", "image/png"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.tar.gz")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))
			s := newRouteServer(t, answerJSON(http.StatusOK, uploadReply))

			_, err := s.client.UploadSource(t.Context(), path)

			require.NoError(t, err)
			req := s.only(t)
			parts := parseForm(t, req.Body, boundaryOf(t, req.Header))
			require.Len(t, parts, 1)
			assert.Equal(t, tc.want, parts[0].ContentType)
			assert.Equal(t, tc.content, string(parts[0].Data))
			assert.Equal(t, int64(len(req.Body)), req.ContentLength)
		})
	}
}

func TestUploadSourceStreamsALargeFile(t *testing.T) {
	path, archive := writeArchive(t, 3<<20)
	s := newRouteServer(t, answerJSON(http.StatusOK, uploadReply))

	_, err := s.client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	req := s.only(t)
	parts := parseForm(t, req.Body, boundaryOf(t, req.Header))
	require.Len(t, parts, 1)
	assert.True(t, bytes.Equal(archive, parts[0].Data))
	assert.Equal(t, int64(len(req.Body)), req.ContentLength)
}

func TestUploadMissingFile(t *testing.T) {
	s := newRouteServer(t, nil)

	_, err := s.client.UploadSource(t.Context(), filepath.Join(t.TempDir(), "missing.tar.gz"))

	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, s.rec.count())
	assert.Empty(t, s.clock.Sleeps())
	assert.Empty(t, s.notices.String())
}

func TestUploadOfADirectoryFails(t *testing.T) {
	s := newRouteServer(t, nil)

	_, err := s.client.UploadSource(t.Context(), t.TempDir())

	require.Error(t, err)
	assert.Zero(t, s.rec.count())
	assert.Empty(t, s.clock.Sleeps())
}

func TestUploadUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not stop this user from reading")
	}
	path, _ := writeArchive(t, 1<<10)
	require.NoError(t, os.Chmod(path, 0))
	s := newRouteServer(t, nil)

	_, err := s.client.UploadSource(t.Context(), path)

	require.ErrorIs(t, err, os.ErrPermission)
	assert.Zero(t, s.rec.count())
	assert.Empty(t, s.clock.Sleeps())
}

func TestUploadResponseWithoutAKeyIsAnEmptyKey(t *testing.T) {
	path, _ := writeArchive(t, 1<<10)
	s := newRouteServer(t, answerJSON(http.StatusOK, `{}`))

	key, err := s.client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	assert.Empty(t, key)
}

func TestUploadUndecodableResponseIsNotRetried(t *testing.T) {
	path, _ := writeArchive(t, 1<<10)
	s := newRouteServer(t, answer(http.StatusOK, "text/plain", "not json"))

	_, err := s.client.UploadSource(t.Context(), path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable response from")
	assert.Equal(t, 1, s.rec.count())
	assert.Empty(t, s.notices.String())
}
