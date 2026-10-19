package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var gzipMagic = [2]byte{0x1f, 0x8b}

// CheckTgz validates a prebuilt --tgz archive the way v2.6.1 does (gzip magic, readable) before it is uploaded as-is.
func CheckTgz(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return unreadable(path, err)
	}
	defer f.Close()
	var magic [2]byte
	_, err = io.ReadFull(f, magic[:])
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF): // fewer than two bytes
		return notGzip(path)
	case err != nil:
		return unreadable(path, err)
	case magic != gzipMagic:
		return notGzip(path)
	}
	return nil
}

func unreadable(path string, err error) error {
	return fmt.Errorf("unable to read compressed file %q: %w", path, err)
}

func notGzip(path string) error {
	return fmt.Errorf("%q is not a valid compressed file", path)
}
