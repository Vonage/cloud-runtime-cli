package platform

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenFile is a file whose reads or seeks fail.
type brokenFile struct {
	data    *strings.Reader
	readErr error
	seekErr error
}

func (b *brokenFile) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.data.Read(p)
}

func (b *brokenFile) Seek(offset int64, whence int) (int64, error) {
	if b.seekErr != nil {
		return 0, b.seekErr
	}
	return b.data.Seek(offset, whence)
}

func TestNewFormWritesTheOpeningAndTheClosing(t *testing.T) {
	f := &brokenFile{data: strings.NewReader("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 2000))}

	got, err := newForm(f, "b0undary")

	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data; boundary=b0undary", got.contentType)
	assert.Equal(t, "--b0undary\r\n"+
		`Content-Disposition: form-data; name="tgz-code"; filename="tgz-code.tar.gz"`+"\r\n"+
		"Content-Type: image/png\r\n\r\n", string(got.opening))
	assert.Equal(t, "\r\n--b0undary--\r\n", string(got.closing))
	rest, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Len(t, rest, 8+2000, "the file is back at its start")
}

func TestNewFormFailures(t *testing.T) {
	boom := errors.New("disk on fire")
	tests := map[string]struct {
		file     *brokenFile
		boundary string
		want     string
	}{
		"a read that fails":      {file: &brokenFile{data: strings.NewReader("x"), readErr: boom}, boundary: "b", want: "disk on fire"},
		"a seek that fails":      {file: &brokenFile{data: strings.NewReader("x"), seekErr: boom}, boundary: "b", want: "disk on fire"},
		"a boundary that is bad": {file: &brokenFile{data: strings.NewReader("x")}, boundary: "", want: "invalid boundary"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newForm(tc.file, tc.boundary)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
