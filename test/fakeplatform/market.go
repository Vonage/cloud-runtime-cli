package fakeplatform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	codeTemplateNotFound = 2010
	msgTemplateNotFound  = "template not found"
	mediaGzip            = "application/gzip"
	modeRegular          = 0o644
	manifestFile         = "vcr.yml"
	minimalManifest      = "project:\n  name: starter-project\ninstance:\n  name: dev\n  runtime: nodejs18\n" +
		"  region: aws.euw1\n  entrypoint:\n    - node\n    - index.js\n"
)

// Template is the source archive of a product's active version. A product
// whose template sets no field serves one file, vcr.yml with a minimal
// manifest. Otherwise the archive holds Files (regular files; their parent
// directories are implied, not listed), then Entries, then the Generate
// files, each named exactly as given. ArchiveFrom instead serves the bytes of
// a file as they are; the path is absolute or relative to the working
// directory, and is read when the server starts.
type Template struct {
	Files       []TemplateFile   `yaml:"files,omitempty"`
	Entries     []TemplateEntry  `yaml:"entries,omitempty"`
	Generate    TemplateGenerate `yaml:"generate,omitempty"`
	ArchiveFrom string           `yaml:"archiveFrom,omitempty"`
}

// TemplateFile is a regular file of a template. Mode is an octal YAML
// integer, written as given (0 stays 0).
type TemplateFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	Mode    int64  `yaml:"mode"`
}

// TemplateEntry is a raw archive entry, whatever its name. Type is file (the
// default), dir, symlink, hardlink, fifo or char; Link is the target of a
// link; Content goes into a file only.
type TemplateEntry struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type,omitempty"`
	Content string `yaml:"content,omitempty"`
	Link    string `yaml:"link,omitempty"`
	Mode    int64  `yaml:"mode"`
}

// TemplateGenerate adds Count empty files named fmt.Sprintf(Pattern, i), i
// counting from 0.
type TemplateGenerate struct {
	Pattern string `yaml:"pattern"`
	Count   int    `yaml:"count"`
}

var entryTypes = map[string]byte{
	"": tar.TypeReg, "file": tar.TypeReg, "dir": tar.TypeDir, "symlink": tar.TypeSymlink,
	"hardlink": tar.TypeLink, "fifo": tar.TypeFifo, "char": tar.TypeChar,
}

func (tm *Template) isZero() bool {
	return len(tm.Files) == 0 && len(tm.Entries) == 0 && tm.Generate.Count == 0 && tm.ArchiveFrom == ""
}

func (tm *Template) validate() error {
	var errs []error
	if tm.ArchiveFrom != "" && (len(tm.Files) > 0 || len(tm.Entries) > 0 || tm.Generate.Count > 0) {
		errs = append(errs, errors.New("archiveFrom excludes files, entries and generate"))
	}
	for i, f := range tm.Files {
		if f.Path == "" {
			errs = append(errs, fmt.Errorf("files[%d] has no path", i))
		}
	}
	for i, e := range tm.Entries {
		if _, ok := entryTypes[e.Type]; !ok {
			errs = append(errs, fmt.Errorf("entries[%d]: unknown type %q", i, e.Type))
		}
	}
	switch g := tm.Generate; {
	case g.Count < 0:
		errs = append(errs, errors.New("generate.count cannot be negative"))
	case g.Count > 0 && !strings.Contains(g.Pattern, "%"):
		errs = append(errs, fmt.Errorf("generate.pattern %q has no verb for the number", g.Pattern))
	}
	return errors.Join(errs...)
}

// archive returns the bytes the template serves.
func (tm *Template) archive() ([]byte, error) {
	if tm.ArchiveFrom != "" {
		b, err := os.ReadFile(tm.ArchiveFrom)
		if err != nil {
			return nil, fmt.Errorf("archiveFrom: %w", err)
		}
		return b, nil
	}
	if tm.isZero() {
		return buildArchive([]tarEntry{{name: manifestFile, typeflag: tar.TypeReg, mode: modeRegular, content: []byte(minimalManifest)}})
	}
	entries := make([]tarEntry, 0, len(tm.Files)+len(tm.Entries)+tm.Generate.Count)
	for _, f := range tm.Files {
		entries = append(entries, tarEntry{name: f.Path, typeflag: tar.TypeReg, mode: f.Mode, content: []byte(f.Content)})
	}
	for _, e := range tm.Entries {
		entries = append(entries, tarEntry{name: e.Name, typeflag: entryTypes[e.Type], mode: e.Mode, link: e.Link, content: []byte(e.Content)})
	}
	for i := range tm.Generate.Count {
		entries = append(entries, tarEntry{name: fmt.Sprintf(tm.Generate.Pattern, i), typeflag: tar.TypeReg, mode: modeRegular})
	}
	return buildArchive(entries)
}

// tarEntry is one entry of an archive the fake builds.
type tarEntry struct {
	name, link string
	typeflag   byte
	mode       int64
	content    []byte
}

// buildArchive writes entries as a deterministic gzip tar: PAX headers with a
// zero modification time, uid and gid 0 and the names exactly as given, in a
// gzip stream with an empty header at the best compression.
func buildArchive(entries []tarEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		hdr := &tar.Header{
			Typeflag: e.typeflag, Name: e.name, Linkname: e.link, Mode: e.mode,
			ModTime: time.Unix(0, 0), Format: tar.FormatPAX,
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("archive entry %q: %w", e.name, err)
		}
		if _, err := tw.Write(e.content[:hdr.Size]); err != nil {
			return nil, fmt.Errorf("archive entry %q: %w", e.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// builtTemplate is the archive of a product's template and the version it belongs to.
type builtTemplate struct {
	version string
	archive []byte
}

// buildTemplates builds the archive of every product, keyed by the product
// id in lower case; the first product with an id wins.
func buildTemplates(products []Product) (map[string]builtTemplate, error) {
	out := make(map[string]builtTemplate, len(products))
	for i := range products {
		p := &products[i]
		key := strings.ToLower(p.ID)
		if _, seen := out[key]; seen {
			continue
		}
		archive, err := p.Template.archive()
		if err != nil {
			return nil, fmt.Errorf("products[%d].template: %w", i, err)
		}
		out[key] = builtTemplate{version: p.ActiveVersionID, archive: archive}
	}
	return out, nil
}

// marketRoute is the template download of the marketplace, under any prefix.
// Its canonical key names both ids {id}.
var marketRoute = func() route {
	rt := newRoute(http.MethodGet, "/products/{product}/versions/{version}/download/source", (*Server).serveTemplate)
	rt.kind, rt.key = kindMarket, http.MethodGet+" "+marketPrefix+"/products/{id}/versions/{id}/download/source"
	return rt
}()

// marketShape matches the end of a template download path.
var marketShape = []string{"products", anySegment, "versions", anySegment, "download", "source"}

// marketPath returns the marketplace path of a template download, from
// /products/ on, whatever comes before it.
func marketPath(path string) (string, bool) {
	segs := splitPath(path)
	if len(segs) < len(marketShape) {
		return "", false
	}
	tail := segs[len(segs)-len(marketShape):]
	if !matchSegments(marketShape, tail) {
		return "", false
	}
	return "/" + strings.Join(tail, "/"), true
}

// serveTemplate answers the archive of the product's active version.
func (s *Server) serveTemplate(c *call) {
	product, okProduct := c.param("product")
	version, okVersion := c.param("version")
	t, found := s.built.templates[strings.ToLower(product)]
	if !okProduct || !okVersion || !found || !strings.EqualFold(t.version, version) {
		writeError(c.rw, http.StatusNotFound, codeTemplateNotFound, msgTemplateNotFound)
		return
	}
	writeRaw(c.rw, http.StatusOK, mediaGzip, t.archive)
}
