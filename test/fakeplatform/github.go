package fakeplatform

import (
	"archive/tar"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	checksumsName    = "checksums.txt"
	mediaOctetStream = "application/octet-stream"
	mismatchedTag    = "v0.0.1"
	wrongNamespace   = "file"
	modeExecutable   = 0o755
	msgCredentials   = "credentials sent to the release host"

	tamperArchiveBitflip   = "archive-bitflip"
	tamperListBitflip      = "list-bitflip"
	tamperSignatureBitflip = "signature-bitflip"
	tamperUnknownKey       = "unknown-key"
	tamperMissingSignature = "missing-signature"
	tamperMissingEntry     = "missing-entry"
	tamperTagMismatch      = "tag-mismatch"
	tamperWrongNamespace   = "wrong-namespace"
)

var (
	releasePlatforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64"}
	tamperModes      = []string{
		tamperArchiveBitflip, tamperListBitflip, tamperSignatureBitflip, tamperUnknownKey,
		tamperMissingSignature, tamperMissingEntry, tamperTagMismatch, tamperWrongNamespace,
	}
	platformShape = regexp.MustCompile(`^[a-z0-9]+_[a-z0-9]+$`)
)

// Release is the latest release of the fake GitHub under /gh: one archive per
// platform (<os>_<arch>, in the order given; empty means the five release
// platforms), checksums.txt and its signature by the trusted test key.
//
// Tamper serves one defect, applied after the pristine checksums were
// computed: archive-bitflip (one bit of this machine's archive), list-bitflip
// (one bit of checksums.txt), signature-bitflip (one bit of the signature),
// unknown-key (the trusted key's signature file is 404, and a valid signature
// by another key is served under that key's id), missing-signature (the
// signature file is 404), missing-entry (no line for this machine's archive,
// validly signed), tag-mismatch (first line "# vcr v0.0.1", validly signed) or
// wrong-namespace (signed in the namespace "file"). This machine is the
// platform of the test process, where the CLI under test runs.
//
// FailOnCredentials, for unit tests, answers 400 to any /gh request that
// carries Authorization or an X-Neru-* header and reports it with Errorf on
// the testing.TB given to Start.
type Release struct {
	Tag               string   `yaml:"tag"`
	Platforms         []string `yaml:"platforms"`
	Tamper            string   `yaml:"tamper,omitempty"`
	FailOnCredentials bool     `yaml:"failOnCredentials,omitempty"`
}

func (r *Release) validate() error {
	var errs []error
	if r.Tag == "" {
		errs = append(errs, errors.New("no tag"))
	}
	for i, p := range r.Platforms {
		if !platformShape.MatchString(p) {
			errs = append(errs, fmt.Errorf("platforms[%d] %q is not <os>_<arch>", i, p))
		}
	}
	if r.Tamper != "" && !slices.Contains(tamperModes, r.Tamper) {
		errs = append(errs, fmt.Errorf("unknown tamper mode %q", r.Tamper))
	}
	return errors.Join(errs...)
}

// release is what the fake GitHub serves: the asset names in listing order
// and the bytes of each. A listed name without bytes answers 404.
type release struct {
	tag    string
	names  []string
	served map[string][]byte
}

func archiveName(platform string) string { return "vcr_" + platform + ".tar.gz" }

func signatureName(pub ed25519.PublicKey) string { return checksumsName + "." + KeyID(pub) + ".sig" }

// hostPlatform is the platform of the test process.
func hostPlatform() string { return runtime.GOOS + "_" + runtime.GOARCH }

// releaseArchive holds one executable named like the archive stem.
func releaseArchive(tag, platform string) ([]byte, error) {
	name := "vcr_" + platform
	if strings.HasPrefix(platform, "windows_") {
		name += ".exe"
	}
	script := fmt.Sprintf("#!/bin/sh\necho \"fake vcr %s %s\"\n", tag, platform)
	return buildArchive([]tarEntry{{name: name, typeflag: tar.TypeReg, mode: modeExecutable, content: []byte(script)}})
}

// checksumList is checksums.txt: the first line names the tag, then one
// "<sha256>  <archive>" line per archive, leaving out the one named skip.
func checksumList(tag string, archives []string, served map[string][]byte, skip string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# vcr %s\n", tag)
	for _, name := range archives {
		if name != skip {
			fmt.Fprintf(&b, "%x  %s\n", sha256.Sum256(served[name]), name)
		}
	}
	return b.Bytes()
}

func buildRelease(r *Release) (release, error) {
	platforms := r.Platforms
	if len(platforms) == 0 {
		platforms = releasePlatforms
	}
	rel := release{tag: r.Tag, served: map[string][]byte{}}
	for _, p := range platforms {
		archive, err := releaseArchive(r.Tag, p)
		if err != nil {
			return release{}, fmt.Errorf("release archive %s: %w", p, err)
		}
		rel.names = append(rel.names, archiveName(p))
		rel.served[archiveName(p)] = archive
	}
	archives := slices.Clone(rel.names)
	list := checksumList(r.Tag, archives, rel.served, "")
	trusted := signatureName(publicKey(trustedKey()))
	rel.names = append(rel.names, checksumsName, trusted)
	rel.served[checksumsName] = list
	rel.served[trusted] = SignSSHSIG(trustedKey(), releaseNamespace, list)
	rel.tamper(r.Tamper, archives, list)
	return rel, nil
}

// tamper applies one tamper mode to a release built from the pristine list.
func (rel *release) tamper(mode string, archives []string, list []byte) {
	trusted := signatureName(publicKey(trustedKey()))
	resign := func(l []byte) {
		rel.served[checksumsName] = l
		rel.served[trusted] = SignSSHSIG(trustedKey(), releaseNamespace, l)
	}
	switch mode {
	case tamperArchiveBitflip:
		if b, ok := rel.served[archiveName(hostPlatform())]; ok {
			rel.served[archiveName(hostPlatform())] = flipBit(b, len(b)/2)
		}
	case tamperListBitflip:
		rel.served[checksumsName] = flipBit(list, len(list)/2)
	case tamperSignatureBitflip:
		blob := sshsigBlob(trustedKey(), releaseNamespace, list)
		rel.served[trusted] = armorSSHSIG(flipBit(blob, len(blob)-1))
	case tamperUnknownKey:
		other := signatureName(publicKey(otherKey()))
		delete(rel.served, trusted)
		rel.names = append(rel.names, other)
		rel.served[other] = SignSSHSIG(otherKey(), releaseNamespace, list)
	case tamperMissingSignature:
		delete(rel.served, trusted)
	case tamperMissingEntry:
		resign(checksumList(rel.tag, archives, rel.served, archiveName(hostPlatform())))
	case tamperTagMismatch:
		resign(checksumList(mismatchedTag, archives, rel.served, ""))
	case tamperWrongNamespace:
		rel.served[trusted] = SignSSHSIG(trustedKey(), wrongNamespace, list)
	}
}

// flipBit returns a copy of b with the lowest bit of byte i flipped; the last
// byte of a signature blob is inside the ed25519 signature, so flipping it
// leaves the blob readable and the signature invalid.
func flipBit(b []byte, i int) []byte {
	out := bytes.Clone(b)
	if len(out) > 0 {
		out[i] ^= 0x01
	}
	return out
}

// githubRoutes are the release routes; their canonical keys name the gh: space.
var githubRoutes = []route{
	githubRoute("/releases/latest", (*Server).serveLatestRelease),
	githubRoute("/download/{tag}/{name}", (*Server).serveReleaseAsset),
}

func githubRoute(pattern string, handle func(*Server, *call)) route {
	rt := newRoute(http.MethodGet, pattern, handle)
	rt.kind, rt.key = kindGitHub, http.MethodGet+" "+ghPrefix+pattern
	return rt
}

type releaseJSON struct {
	TagName string      `json:"tag_name"`
	Assets  []assetJSON `json:"assets"`
}

type assetJSON struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (s *Server) serveLatestRelease(c *call) {
	rel := &s.built.release
	out := releaseJSON{TagName: rel.tag, Assets: []assetJSON{}}
	for _, name := range rel.names {
		u := s.url + githubPrefix + "/download/" + url.PathEscape(rel.tag) + "/" + url.PathEscape(name)
		out.Assets = append(out.Assets, assetJSON{Name: name, BrowserDownloadURL: u})
	}
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) serveReleaseAsset(c *call) {
	rel := &s.built.release
	tag, okTag := c.param("tag")
	name, okName := c.param("name")
	body, found := rel.served[name]
	if !okTag || !okName || tag != rel.tag || !found {
		writeRaw(c.rw, http.StatusNotFound, mediaText, []byte(http.StatusText(http.StatusNotFound)))
		return
	}
	c.rw.Header().Set("Content-Length", strconv.Itoa(len(body)))
	writeRaw(c.rw, http.StatusOK, mediaOctetStream, body)
}

// refuseCredentials answers 400, and fails the test, when a request to the
// release host carries platform credentials.
func (s *Server) refuseCredentials(rw http.ResponseWriter, r *http.Request) bool {
	var sent []string
	for name := range r.Header {
		if strings.EqualFold(name, "Authorization") || strings.HasPrefix(strings.ToLower(name), "x-neru-") {
			sent = append(sent, name)
		}
	}
	if len(sent) == 0 {
		return false
	}
	slices.Sort(sent)
	s.tb.Errorf("fakeplatform: %s %s carried credentials to the release host: %s", r.Method, r.URL.EscapedPath(), strings.Join(sent, ", "))
	writeError(rw, http.StatusBadRequest, http.StatusBadRequest, msgCredentials)
	return true
}
