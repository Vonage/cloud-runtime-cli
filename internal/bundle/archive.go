package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
)

const tempPattern = "vcr-upload-*.tar.gz"

// ctxReader fails a copy as soon as its context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// archiver writes the entries the walk hands it into a tar stream.
type archiver struct {
	ctx   context.Context
	tw    *tar.Writer
	files int
}

func (a *archiver) add(e Entry, hdr *tar.Header, f fs.File) error {
	if f != nil {
		defer func() { _ = f.Close() }()
	}
	if err := a.tw.WriteHeader(hdr); err != nil {
		return addError(e.Path, err)
	}
	if f != nil {
		// CopyN keeps a file that grew since the header was made from writing more than the header says.
		if _, err := io.CopyN(a.tw, ctxReader{ctx: a.ctx, r: f}, hdr.Size); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("%s changed while it was being packaged: %w", e.Path, io.ErrUnexpectedEOF)
			}
			return addError(e.Path, err)
		}
	}
	a.files++
	return nil
}

func addError(name string, err error) error {
	return fmt.Errorf("failed to add %s to the archive: %w", name, err)
}

// Build packages root into a tar.gz (gzip level 1) in a new temp file and returns its path and file count.
// cleanup removes the temp file; Build itself removes it on every error path, including cancellation.
func Build(ctx context.Context, root string, warn io.Writer) (path string, files int, cleanup func(), err error) {
	return build(ctx, root, warn, finish)
}

// endFunc ends the archive: the tar stream, the gzip stream and the file.
type endFunc func(tw *tar.Writer, gz *gzip.Writer, out *os.File) error

// build is Build with the last step passed in, so that a test can make it fail.
func build(ctx context.Context, root string, warn io.Writer, end endFunc) (path string, files int, cleanup func(), err error) {
	noCleanup := func() {}
	w, err := newWalker(root, warn)
	if err != nil {
		return "", 0, noCleanup, err
	}
	out, err := os.CreateTemp("", tempPattern)
	if err != nil {
		return "", 0, noCleanup, fmt.Errorf("failed to create a temporary file for the upload: %w", err)
	}
	gz, _ := gzip.NewWriterLevel(out, gzip.BestSpeed) // only an invalid level fails
	a := &archiver{ctx: ctx, tw: tar.NewWriter(gz)}
	if err = w.walk(ctx, a.add); err != nil {
		discard(out)
		return "", 0, noCleanup, err
	}
	if err = end(a.tw, gz, out); err != nil {
		discard(out)
		return "", 0, noCleanup, fmt.Errorf("failed to write the archive: %w", err)
	}
	name := out.Name()
	return name, a.files, sync.OnceFunc(func() { _ = os.Remove(name) }), nil
}

// finish ends the tar stream, then the gzip stream, then the file, so that a full disk shows up as an error.
func finish(tw *tar.Writer, gz *gzip.Writer, out *os.File) error {
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return out.Close()
}

// discard closes and removes a temporary file that will not be used.
func discard(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}
