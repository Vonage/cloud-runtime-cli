//go:build !windows

package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

const (
	kindFile  = "file"
	kindDir   = "dir"
	kindLink  = "link"
	kindFifo  = "fifo"
	kindOther = "other"

	maxKeptBytes   = 1 << 20 // text files up to this size keep their content for the diff; larger ones are not normalised and keep the raw-byte checksum
	maxInlineBytes = 4096    // text files up to this size are shown in the files section
)

// snapEntry is what a snapshot knows about one path.
type snapEntry struct {
	kind    string // file, dir, link or the name of another type
	perm    os.FileMode
	size    int64
	sum     string // sha256 of the raw bytes of a file
	link    string
	content []byte // set for files up to maxKeptBytes
}

// Snapshot is the state of a whole sandbox: no times, only kind, mode and content.
type Snapshot struct{ entries map[string]snapEntry }

// TakeSnapshot walks root and records every path below it.
func TakeSnapshot(root string) (Snapshot, error) {
	s := Snapshot{entries: map[string]snapEntry{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		e, err := entryOf(p, info)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		s.entries[filepath.ToSlash(rel)] = e
		return nil
	})
	return s, err
}

func entryOf(p string, info fs.FileInfo) (snapEntry, error) {
	e := snapEntry{perm: info.Mode().Perm(), size: info.Size()}
	switch t := info.Mode().Type(); {
	case t == 0:
		e.kind = kindFile
		return readFileEntry(p, e)
	case t.IsDir():
		e.kind, e.size = kindDir, 0
	case t&fs.ModeSymlink != 0:
		target, err := os.Readlink(p)
		if err != nil {
			return e, err
		}
		e.kind, e.link, e.size = kindLink, target, 0
	case t&fs.ModeNamedPipe != 0:
		e.kind = kindFifo
	case t&fs.ModeSocket != 0:
		e.kind = "socket"
	case t&fs.ModeDevice != 0:
		e.kind = "device"
	default:
		e.kind = kindOther
	}
	return e, nil
}

// readFileEntry hashes a regular file and keeps its content when it is small.
func readFileEntry(p string, e snapEntry) (snapEntry, error) {
	f, err := os.Open(p)
	if err != nil {
		e.sum = "unreadable"
		return e, nil //nolint:nilerr // a file the run made unreadable is still a file
	}
	defer f.Close()
	h := sha256.New()
	var kept bytes.Buffer
	var w io.Writer = h
	if e.size <= maxKeptBytes {
		w = io.MultiWriter(h, &kept)
	}
	n, err := io.Copy(w, f)
	if err != nil {
		return e, err
	}
	e.size, e.sum = n, hex.EncodeToString(h.Sum(nil))
	if e.size <= maxKeptBytes {
		e.content = kept.Bytes()
	}
	return e, nil
}

func (e snapEntry) isText() bool {
	return e.kind == kindFile && e.content != nil && utf8.Valid(e.content) && !bytes.Contains(e.content, []byte{0})
}

func (e snapEntry) displayPath(p string) string {
	if e.kind == kindDir {
		return p + "/"
	}
	return p
}

func modeText(m os.FileMode) string { return fmt.Sprintf("%04o", m) }

// DiffSnapshots lists what a run created, changed and deleted, sorted by
// path. The size and checksum of a text file are those of its normalised
// text, so that a path or port inside it does not make the run differ.
func DiffSnapshots(before, after Snapshot, n Normaliser) []FileChange {
	var out []FileChange
	for p, a := range after.entries {
		b, existed := before.entries[p]
		switch {
		case !existed:
			out = append(out, describe("+", p, a, "", n))
		case b.kind != a.kind:
			out = append(out, FileChange{Op: "-", Path: b.displayPath(p)}, describe("+", p, a, "", n))
		default:
			if c, changed := changeOf(p, b, a, n); changed {
				out = append(out, c)
			}
		}
	}
	for p, b := range before.entries {
		if _, kept := after.entries[p]; !kept {
			out = append(out, FileChange{Op: "-", Path: b.displayPath(p)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// changeOf describes how an entry that exists on both sides changed.
func changeOf(p string, b, a snapEntry, n Normaliser) (FileChange, bool) {
	switch {
	case a.kind == kindLink:
		if a.link == b.link {
			return FileChange{}, false
		}
		return describe("~", p, a, "", n), true
	case a.perm == b.perm && a.sum == b.sum:
		return FileChange{}, false
	}
	mode := ""
	if a.perm != b.perm {
		mode = modeText(b.perm) + "->" + modeText(a.perm)
	}
	if a.kind == kindFile && a.sum == b.sum {
		return FileChange{Op: "~", Path: p, Mode: mode}, true
	}
	return describe("~", p, a, mode, n), true
}

// describe renders an entry that exists after the run.
func describe(op, p string, e snapEntry, mode string, n Normaliser) FileChange {
	if mode == "" {
		mode = modeText(e.perm)
	}
	c := FileChange{Op: op, Path: e.displayPath(p), Mode: mode}
	switch e.kind {
	case kindLink:
		c.Link = n.Text(e.link)
	case kindFile:
		c.Size, c.SHA = e.size, e.sum
		if e.isText() {
			text := n.Text(string(e.content))
			sum := sha256.Sum256([]byte(text))
			c.Size, c.SHA, c.Text = int64(len(text)), hex.EncodeToString(sum[:]), text
			c.Inline = len(e.content) <= maxInlineBytes
		}
	case kindDir:
	default:
		c.Type = e.kind
	}
	return c
}
