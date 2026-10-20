package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

// maxTrailingBytes is how much padding may follow the end of the tar inside the gzip stream. It is read only so that
// gzip can check its own trailer.
const maxTrailingBytes = 1 << 20

// archiveError is an archive that is not a gzip-compressed tar holding exactly one regular file within the size
// limit. Its text is the reason the user sees; the cause says which rule it broke.
type archiveError struct{ cause error }

func (e *archiveError) Error() string { return "the archive does not contain exactly one file" }
func (e *archiveError) Unwrap() error { return e.cause }

// extractSingle returns the one regular file of the gzip-compressed tar archive. Entry names carry no meaning and
// nothing is created from them. A second entry, an entry that is not a regular file, a file of more than limit
// bytes or of none, an empty archive and damaged gzip or tar all fail with an *archiveError. At most limit+1 bytes are read
// from the file, so a decompression bomb stops at the limit.
func extractSingle(archive []byte, limit int64) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, &archiveError{cause: err}
	}
	defer func() { _ = zr.Close() }()

	tr := tar.NewReader(zr)
	hdr, err := tr.Next()
	switch {
	case err != nil:
		return nil, &archiveError{cause: err}
	case hdr.Typeflag != tar.TypeReg:
		return nil, &archiveError{cause: errors.New("the only entry is not a regular file")}
	case hdr.Size > limit:
		return nil, &archiveError{cause: errTooLarge}
	}

	body, err := io.ReadAll(io.LimitReader(tr, limit+1))
	switch {
	case err != nil:
		return nil, &archiveError{cause: err}
	case int64(len(body)) > limit:
		return nil, &archiveError{cause: errTooLarge}
	case len(body) == 0:
		return nil, &archiveError{cause: errors.New("the file is empty")}
	}
	if err := checkEnd(tr, zr); err != nil {
		return nil, err
	}
	return body, nil
}

// checkEnd requires that the tar ends after its first entry, and reads the gzip stream to its end so that the
// trailer is checked.
func checkEnd(tr *tar.Reader, zr io.Reader) error {
	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("the archive has a second entry")
		}
		return &archiveError{cause: err}
	}
	n, err := io.CopyN(io.Discard, zr, maxTrailingBytes+1)
	switch {
	case err != nil && !errors.Is(err, io.EOF):
		return &archiveError{cause: fmt.Errorf("the gzip stream is damaged: %w", err)}
	case n > maxTrailingBytes:
		return &archiveError{cause: errors.New("the archive has data after its end")}
	}
	return nil
}
