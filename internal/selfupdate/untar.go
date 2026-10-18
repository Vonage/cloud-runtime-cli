package selfupdate

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

// extractSingle writes the one regular file of the gzip-compressed tar archive to dst. Entry names carry no meaning
// and nothing is created from them: dst is all there is to write to. A second entry, an entry that is not a regular
// file, a file of more than limit bytes, an empty archive and damaged gzip or tar all fail with an *archiveError;
// a failing dst is returned as it failed. At most limit+1 bytes reach dst, so a decompression bomb stops at the limit.
func extractSingle(archive []byte, dst io.Writer, limit int64) error {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return &archiveError{cause: err}
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	hdr, err := tr.Next()
	switch {
	case err != nil:
		return &archiveError{cause: err}
	case hdr.Typeflag != tar.TypeReg:
		return &archiveError{cause: errors.New("the only entry is not a regular file")}
	case hdr.Size > limit:
		return &archiveError{cause: errTooLarge}
	}

	sink := &writeRecorder{w: dst}
	n, err := io.CopyN(sink, tr, limit+1)
	switch {
	case sink.err != nil:
		return sink.err
	case err != nil && !errors.Is(err, io.EOF):
		return &archiveError{cause: err}
	case n > limit:
		return &archiveError{cause: errTooLarge}
	}
	return checkEnd(tr, zr)
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

// writeRecorder remembers the error its writer returned, to tell a destination that failed from an archive that is
// bad: io.CopyN reports both the same way.
type writeRecorder struct {
	w   io.Writer
	err error
}

func (r *writeRecorder) Write(p []byte) (int, error) {
	n, err := r.w.Write(p)
	if err != nil && r.err == nil {
		r.err = err
	}
	return n, err
}
