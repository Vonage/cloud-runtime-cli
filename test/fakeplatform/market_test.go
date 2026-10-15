package fakeplatform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const marketplacePrefix = "/v1/vcr/euw1/api/marketplace"

var (
	productID = fixtureID(201)
	versionID = fixtureID(301)
)

// archived is one entry of an archive read back.
type archived struct {
	hdr     *tar.Header
	content string
}

// readArchive gunzips and untars gz; insecure names are read like any other.
func readArchive(t *testing.T, gz []byte) []archived {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(t, err)
	tr := tar.NewReader(zr)
	var out []archived
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if !errors.Is(err, tar.ErrInsecurePath) {
			require.NoError(t, err)
		}
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		out = append(out, archived{hdr: hdr, content: string(body)})
	}
}

func names(entries []archived) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.hdr.Name
	}
	return out
}

func templateURL(s *Server, prefix, product, version string) string {
	return s.URL() + prefix + "/products/" + product + "/versions/" + version + "/download/source"
}

func downloadTemplate(t *testing.T, s *Server, product, version string) reply {
	t.Helper()
	return send(t, http.MethodGet, templateURL(s, marketplacePrefix, product, version), nil, asAccount)
}

// templateFixtures replaces the default product with one carrying the template given as YAML.
func templateFixtures(template string) string {
	return "products:\n  - id: " + productID + "\n    name: Starter Project - Node.js\n    programmingLanguage: nodejs\n" +
		"    activeVersionId: " + versionID + "\n    template:\n" + template
}

func TestTemplateDownloadServesActiveVersion(t *testing.T) {
	s := Start(t, Defaults())
	r := downloadTemplate(t, s, productID, versionID)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	assert.Equal(t, "application/gzip", r.header.Get("Content-Type"))
	entries := readArchive(t, r.body)
	require.Len(t, entries, 1, "without a template field the product serves one file")
	assert.Equal(t, "vcr.yml", entries[0].hdr.Name)
	assert.Equal(t, byte(tar.TypeReg), entries[0].hdr.Typeflag)
	assert.Equal(t, int64(0o644), entries[0].hdr.Mode)
	assert.Contains(t, entries[0].content, "project:")
	assert.Contains(t, entries[0].content, "instance:")

	req := s.Requests()[0]
	assert.Equal(t, "market", req.Kind)
	assert.Equal(t, "GET market:/products/{id}/versions/{id}/download/source", req.Route)
	assert.Equal(t, "/products/"+productID+"/versions/"+versionID+"/download/source", req.Rel)
	assert.Equal(t, "euw1", req.Region)

	files := started(t, templateFixtures(`      files:
        - {path: vcr.yml, content: "project:\n  name: x\n", mode: 0600}
        - {path: src/index.js, content: "console.log(1)", mode: 0755}
`))
	prefixed := send(t, http.MethodGet, templateURL(files, "/any/prefix", productID, versionID), nil, asAccount)
	require.Equal(t, http.StatusOK, prefixed.status, "any prefix reaches the marketplace route: %s", prefixed.body)
	got := readArchive(t, prefixed.body)
	assert.Equal(t, []string{"vcr.yml", "src/index.js"}, names(got), "only the files, no implied directories")
	require.Len(t, got, 2)
	assert.Equal(t, int64(0o600), got[0].hdr.Mode)
	assert.Equal(t, "project:\n  name: x\n", got[0].content)
	assert.Equal(t, int64(0o755), got[1].hdr.Mode)
	assert.Equal(t, "console.log(1)", got[1].content)
	for _, e := range got {
		assert.Equal(t, int64(0), e.hdr.ModTime.Unix(), e.hdr.Name)
		assert.Equal(t, 0, e.hdr.Uid, e.hdr.Name)
		assert.Equal(t, 0, e.hdr.Gid, e.hdr.Name)
	}
}

func TestTemplateBuilderIsDeterministic(t *testing.T) {
	tmpl := Template{
		Files:    []TemplateFile{{Path: "vcr.yml", Content: "project: {}\n", Mode: 0o644}},
		Entries:  []TemplateEntry{{Name: "dir", Type: "dir", Mode: 0o755}},
		Generate: TemplateGenerate{Pattern: "bulk/f%d", Count: 3},
	}
	first, err := tmpl.archive()
	require.NoError(t, err)
	second, err := tmpl.archive()
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, []string{"vcr.yml", "dir", "bulk/f0", "bulk/f1", "bulk/f2"}, names(readArchive(t, first)),
		"files, then raw entries, then generated files")

	a := downloadTemplate(t, Start(t, Defaults()), productID, versionID).body
	b := downloadTemplate(t, Start(t, Defaults()), productID, versionID).body
	assert.Equal(t, a, b, "two servers serve the same bytes")
	zr, err := gzip.NewReader(bytes.NewReader(a))
	require.NoError(t, err)
	assert.Empty(t, zr.Name)
	assert.Empty(t, zr.Comment)
	assert.Empty(t, zr.Extra)
	assert.True(t, zr.ModTime.IsZero(), "an empty gzip header")
}

func TestTemplateEntriesMayBeHostile(t *testing.T) {
	s := started(t, templateFixtures(`      files: [{path: vcr.yml, content: x, mode: 0644}]
      entries:
        - {name: ../evil, type: file, content: x, mode: 0644}
        - {name: /abs/evil, type: file, content: y, mode: 0600}
        - {name: link, type: symlink, link: /etc}
        - {name: .git/config, type: file, content: "[core]", mode: 0644}
        - {name: hard, type: hardlink, link: vcr.yml}
        - {name: pipe, type: fifo, mode: 0644}
        - {name: dev, type: char, mode: 0644}
        - {name: dir, type: dir, mode: 0755}
        - {name: ok/../fine.txt, content: z, mode: 0}
`))
	r := downloadTemplate(t, s, productID, versionID)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	got := readArchive(t, r.body)
	assert.Equal(t, []string{"vcr.yml", "../evil", "/abs/evil", "link", ".git/config", "hard", "pipe", "dev", "dir", "ok/../fine.txt"}, names(got),
		"names exactly as given")
	require.Len(t, got, 10)
	types := map[string]byte{}
	for _, e := range got {
		types[e.hdr.Name] = e.hdr.Typeflag
	}
	assert.Equal(t, map[string]byte{
		"vcr.yml": tar.TypeReg, "../evil": tar.TypeReg, "/abs/evil": tar.TypeReg, "link": tar.TypeSymlink,
		".git/config": tar.TypeReg, "hard": tar.TypeLink, "pipe": tar.TypeFifo, "dev": tar.TypeChar, "dir": tar.TypeDir,
		"ok/../fine.txt": tar.TypeReg,
	}, types)
	assert.Equal(t, "x", got[1].content)
	assert.Equal(t, int64(0o600), got[2].hdr.Mode)
	assert.Equal(t, "/etc", got[3].hdr.Linkname)
	assert.Equal(t, "[core]", got[4].content)
	assert.Equal(t, "vcr.yml", got[5].hdr.Linkname)
	assert.Equal(t, int64(0), got[9].hdr.Mode, "a mode of 0 is kept")
	assert.Equal(t, "z", got[9].content, "an entry without a type is a file")
}

func TestTemplateGenerateBulkEntries(t *testing.T) {
	s := started(t, templateFixtures("      generate: {pattern: \"bulk/f%05d.txt\", count: 10001}\n"))
	r := downloadTemplate(t, s, productID, versionID)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	got := readArchive(t, r.body)
	require.Len(t, got, 10001)
	assert.Equal(t, "bulk/f00000.txt", got[0].hdr.Name)
	assert.Equal(t, "bulk/f10000.txt", got[10000].hdr.Name)
	for _, e := range got {
		if e.hdr.Typeflag != tar.TypeReg || e.hdr.Size != 0 {
			t.Fatalf("%s is not an empty regular file", e.hdr.Name)
		}
	}
}

func TestTemplateArchiveFromServesRawBytes(t *testing.T) {
	raw := []byte("raw bytes, not even gzip\x00\x01\xff")
	path := filepath.Join(t.TempDir(), "template.tar.gz")
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	f := Defaults()
	f.Products[0].Template = Template{ArchiveFrom: path}
	r := downloadTemplate(t, Start(t, f), productID, versionID)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, raw, r.body)
	assert.Equal(t, "application/gzip", r.header.Get("Content-Type"))

	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			f.Products[0].Template = Template{ArchiveFrom: rel}
			assert.Equal(t, raw, downloadTemplate(t, Start(t, f), productID, versionID).body, "relative to the working directory")
		}
	}

	f.Products[0].Template = Template{ArchiveFrom: filepath.Join(t.TempDir(), "missing.tar.gz")}
	_, err := start(t, "127.0.0.1:0", f)
	require.Error(t, err, "a missing archive fails at start")
	assert.Contains(t, err.Error(), "missing.tar.gz")
}

func TestTemplateUnknownVersionIs404(t *testing.T) {
	s := Start(t, Defaults())
	for _, c := range []struct{ product, version string }{
		{productID, fixtureID(399)},
		{fixtureID(299), versionID},
	} {
		r := downloadTemplate(t, s, c.product, c.version)
		require.Equal(t, http.StatusNotFound, r.status, "body: %s", r.body)
		code, msg := r.apiError(t)
		assert.Equal(t, 2010, code)
		assert.Equal(t, "template not found", msg)
	}
}

func TestTemplateRequiresAuth(t *testing.T) {
	s := Start(t, Defaults())
	for name, opts := range map[string][]option{
		"no credentials":         nil,
		"wrong secret":           {withCredentials(testKey, "wrong", testKey)},
		"missing account header": {withCredentials(testKey, testSecret, "")},
	} {
		r := send(t, http.MethodGet, templateURL(s, marketplacePrefix, productID, versionID), nil, opts...)
		require.Equal(t, http.StatusUnauthorized, r.status, name)
		code, _ := r.apiError(t)
		assert.Equal(t, http.StatusUnauthorized, code, name)
	}
}

func TestMarketOverride(t *testing.T) {
	s := started(t, "overrides: [{match: 'GET market:/products/*/versions/*/download/source', status: 500, body: boom}]\n")
	r := downloadTemplate(t, s, productID, versionID)
	assert.Equal(t, http.StatusInternalServerError, r.status)
	assert.Equal(t, "boom", string(r.body))
	prefixed := send(t, http.MethodGet, templateURL(s, "/elsewhere", productID, versionID), nil)
	assert.Equal(t, http.StatusInternalServerError, prefixed.status, "whatever the prefix")
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/products/templates", nil).status, "the read route is not the marketplace")
}
