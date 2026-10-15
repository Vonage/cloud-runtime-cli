//go:build !windows

package contract

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"strings"
)

const maxUploadBytes = 256 << 20

// renderMultipart renders the parts of a multipart body without their raw
// bytes: tar and gzip bytes legitimately differ between implementations, so a
// gzip tar part is listed by its entries and any other part by its checksum.
func renderMultipart(body []byte, boundary string) string {
	var b strings.Builder
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return b.String()
		}
		if err != nil {
			return b.String() + "  error: " + err.Error() + "\n"
		}
		data, err := io.ReadAll(io.LimitReader(part, maxUploadBytes))
		if err != nil {
			return b.String() + "  error: " + err.Error() + "\n"
		}
		fmt.Fprintf(&b, "  part: name=%s filename=%s\n", part.FormName(), part.FileName())
		if entries, ok := tarEntries(data); ok {
			fmt.Fprintf(&b, "  tar: %d entries\n", len(entries))
			for _, e := range entries {
				b.WriteString("    " + e + "\n")
			}
		} else {
			b.WriteString("  raw: " + checksumText(data) + "\n")
		}
	}
}

type tarLine struct{ name, text string }

// tarEntries lists the entries of a gzip tar, sorted by name, or reports
// false when data is not one.
func tarEntries(data []byte) ([]string, bool) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return nil, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	tr := tar.NewReader(zr)
	var lines []tarLine
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false
		}
		lines = append(lines, tarLine{h.Name, tarEntryText(h)})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].name < lines[j].name })
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
	}
	return out, true
}

func tarEntryText(h *tar.Header) string {
	kind := kindOther
	switch h.Typeflag {
	case tar.TypeReg:
		kind = kindFile
	case tar.TypeDir:
		kind = kindDir
	case tar.TypeSymlink:
		kind = "symlink"
	case tar.TypeLink:
		kind = "hardlink"
	case tar.TypeFifo:
		kind = kindFifo
	}
	text := fmt.Sprintf("%s %04o %d %s", kind, h.Mode&0o7777, h.Size, h.Name)
	if kind == "symlink" || kind == "hardlink" {
		text += " -> " + h.Linkname
	}
	return text
}
