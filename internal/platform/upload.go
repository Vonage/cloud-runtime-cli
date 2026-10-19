package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
)

const (
	uploadPartName    = "tgz-code"
	uploadFileName    = "tgz-code.tar.gz"
	uploadDisposition = `form-data; name="` + uploadPartName + `"; filename="` + uploadFileName + `"`
	sniffBytes        = 512 // how much of the file http.DetectContentType looks at
)

// UploadSource posts the tar.gz at path as multipart part "tgz-code" (filename "tgz-code.tar.gz") to
// POST /packages/source and returns the sourceCodeKey. It retries up to three times (1 s, 2 s, 4 s, ±20% jitter)
// on transport errors and 502/503/504, reopening the file each time and printing one notice line on Notices.
func (c *Client) UploadSource(ctx context.Context, path string) (sourceCodeKey string, err error) {
	// One boundary for every attempt, so that a retry sends the same bytes.
	boundary := multipart.NewWriter(io.Discard).Boundary()
	for retry := 0; ; retry++ {
		sourceCodeKey, err = c.uploadOnce(ctx, path, boundary)
		if err == nil {
			return sourceCodeKey, nil
		}
		reason, again := retryReason(ctx, err)
		if !again || retry == len(uploadBackoff) {
			return "", err
		}
		if err = c.waitToRetry(ctx, retry, reason); err != nil {
			return "", err
		}
	}
}

// uploadOnce makes one attempt: it opens the file afresh, so that a retry sees the file as it is by then, and
// streams it as the one part of a multipart form.
func (c *Client) uploadOnce(ctx context.Context, path, boundary string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer closeBody(f)

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	form, err := newForm(f, boundary)
	if err != nil {
		return "", err
	}

	// The length is declared, so the request is not chunked. The body has no GetBody: an upload is repeated by
	// this function, never by net/http.
	length := int64(len(form.opening)) + info.Size() + int64(len(form.closing))
	body := io.MultiReader(bytes.NewReader(form.opening), f, bytes.NewReader(form.closing))
	header := http.Header{"Content-Type": {form.contentType}}
	rep, err := c.core.send(ctx, http.MethodPost, endpoint(c.base, "packages", "source"), header, body, length)
	if err != nil {
		return "", err
	}
	defer closeBody(rep.Body)

	var out struct {
		SourceCodeKey string `json:"sourceCodeKey"`
	}
	if err := rep.decode(&out); err != nil {
		return "", err
	}
	return out.SourceCodeKey, nil
}

// uploadForm is what surrounds the file in the body of an upload.
type uploadForm struct {
	contentType string // of the request, with the boundary
	opening     []byte // the opening delimiter and the headers of the part
	closing     []byte // the closing delimiter, what multipart.Writer.Close would write
}

// newForm reads the start of f to find its content type, puts f back at its start and writes the part headers.
func newForm(f io.ReadSeeker, boundary string) (uploadForm, error) {
	sniff := make([]byte, sniffBytes)
	n, err := io.ReadFull(f, sniff)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return uploadForm{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return uploadForm{}, err
	}

	var opening bytes.Buffer
	w := multipart.NewWriter(&opening)
	if err := w.SetBoundary(boundary); err != nil {
		return uploadForm{}, err
	}
	_, err = w.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {uploadDisposition},
		"Content-Type":        {http.DetectContentType(sniff[:n])},
	})
	if err != nil {
		return uploadForm{}, err
	}
	return uploadForm{
		contentType: w.FormDataContentType(),
		opening:     opening.Bytes(),
		closing:     []byte("\r\n--" + boundary + "--\r\n"),
	}, nil
}
