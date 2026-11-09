package initcmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	kindInput  = "input"
	kindSelect = "select"

	methodListRuntimes      = "ListRuntimes"
	methodListRegions       = "ListRegions"
	methodListApplications  = "ListApplications"
	methodCreateApplication = "CreateApplication"
	methodListTemplates     = "ListTemplates"
	methodDownloadTemplate  = "DownloadTemplate"
)

var (
	errBoom = errors.New("boom")
	errStop = errors.New("stop here")
)

// textError is an error with any text, for text a real error would carry.
type textError struct{ text string }

func (e *textError) Error() string { return e.text }

// ask is one question the scripted prompter expects, with the answer it gives.
type ask struct {
	kind    string
	message string
	def     string
	answer  string
	prefix  bool     // a select answer is the first option that starts with answer
	options []string // when set, a select must offer exactly these options
	err     error    // when set, the question fails with it
}

func input(message, def, answer string) ask {
	return ask{kind: kindInput, message: message, def: def, answer: answer}
}

func choose(message, def, answer string, options ...string) ask {
	return ask{kind: kindSelect, message: message, def: def, answer: answer, options: options}
}

// pick is a select with no default whose options are not checked.
func pick(message, answer string) ask {
	return ask{kind: kindSelect, message: message, answer: answer}
}

// failing turns a into a question that fails with err.
func (a ask) failing(err error) ask {
	a.err = err
	return a
}

// byPrefix makes a select answer the first option that starts with the answer.
func (a ask) byPrefix() ask {
	a.prefix = true
	return a
}

// script is a term.Prompter that expects exactly the questions it was given, in order. Any difference in kind,
// message or default fails the test.
type script struct {
	t       *testing.T
	asks    []ask
	next    int
	offered [][]string // the options of each select, in order
}

func newScript(t *testing.T, asks ...ask) *script {
	t.Helper()
	s := &script{t: t, asks: asks}
	t.Cleanup(func() {
		if s.next < len(s.asks) {
			t.Errorf("%d scripted questions were never asked, the first being %q", len(s.asks)-s.next, s.asks[s.next].message)
		}
	})
	return s
}

func (s *script) take(kind, message, def string) ask {
	s.t.Helper()
	if s.next >= len(s.asks) {
		s.t.Fatalf("unexpected %s %q (default %q): the script has no more questions", kind, message, def)
	}
	want := s.asks[s.next]
	s.next++
	require.Equal(s.t, want.kind, kind, "question %d: %s", s.next, message)
	require.Equal(s.t, want.message, message, "question %d", s.next)
	require.Equal(s.t, want.def, def, "default of question %d: %s", s.next, message)
	return want
}

func (s *script) Input(message, defaultValue string) (string, error) {
	s.t.Helper()
	a := s.take(kindInput, message, defaultValue)
	return a.answer, a.err
}

func (s *script) Select(message string, options []string, defaultValue string) (string, error) {
	s.t.Helper()
	a := s.take(kindSelect, message, defaultValue)
	s.offered = append(s.offered, slices.Clone(options))
	if a.options != nil {
		require.Equal(s.t, a.options, options, "options of question %d: %s", s.next, message)
	}
	if a.err != nil {
		return "", a.err
	}
	if a.prefix {
		for _, option := range options {
			if strings.HasPrefix(option, a.answer) {
				return option, nil
			}
		}
		s.t.Fatalf("no option of %q starts with %q: %q", message, a.answer, options)
	}
	require.Contains(s.t, options, a.answer, "question %d: %s", s.next, message)
	return a.answer, nil
}

func (s *script) Password(message string, _ bool) (string, error) {
	s.t.Helper()
	s.t.Fatalf("init must not ask for a password: %q", message)
	return "", nil
}

func (s *script) YesNo(question string) (bool, error) {
	s.t.Helper()
	s.t.Fatalf("init must not ask a yes/no question: %q", question)
	return false, nil
}

var _ term.Prompter = (*script)(nil)

// fakeAPI is the platform as init sees it. It records every call.
type fakeAPI struct {
	runtimes  []platform.Runtime
	regions   []platform.Region
	apps      []platform.Application
	templates []platform.Template
	archive   []byte

	failures     map[string]error // a method that always fails
	listAppsErrs []error          // the error of the n-th ListApplications call; nil means success
	createErrs   []error          // the error of the n-th CreateApplication call; nil means success

	calls     []string
	filters   []string
	created   []createdApp
	downloads []download
}

type createdApp struct {
	name                 string
	rtc, voice, messages bool
}

type download struct{ product, version string }

func (f *fakeAPI) record(method string) error {
	f.calls = append(f.calls, method)
	return f.failures[method]
}

func (f *fakeAPI) count(method string) int {
	n := 0
	for _, call := range f.calls {
		if call == method {
			n++
		}
	}
	return n
}

func (f *fakeAPI) ListRuntimes(context.Context) ([]platform.Runtime, error) {
	if err := f.record(methodListRuntimes); err != nil {
		return nil, err
	}
	return slices.Clone(f.runtimes), nil
}

func (f *fakeAPI) ListRegions(context.Context) ([]platform.Region, error) {
	if err := f.record(methodListRegions); err != nil {
		return nil, err
	}
	return slices.Clone(f.regions), nil
}

func (f *fakeAPI) ListApplications(_ context.Context, filter string) ([]platform.Application, error) {
	call := f.count(methodListApplications)
	if err := f.record(methodListApplications); err != nil {
		return nil, err
	}
	f.filters = append(f.filters, filter)
	if call < len(f.listAppsErrs) && f.listAppsErrs[call] != nil {
		return nil, f.listAppsErrs[call]
	}
	return slices.Clone(f.apps), nil
}

func (f *fakeAPI) CreateApplication(_ context.Context, name string, rtc, voice, messages bool) (platform.CreatedApplication, error) {
	call := f.count(methodCreateApplication)
	if err := f.record(methodCreateApplication); err != nil {
		return platform.CreatedApplication{}, err
	}
	f.created = append(f.created, createdApp{name: name, rtc: rtc, voice: voice, messages: messages})
	if call < len(f.createErrs) && f.createErrs[call] != nil {
		return platform.CreatedApplication{}, f.createErrs[call]
	}
	id := "id-of-" + name
	f.apps = append(f.apps, platform.Application{ID: id, Name: name})
	return platform.CreatedApplication{ApplicationID: id, ApplicationName: name}, nil
}

func (f *fakeAPI) ListTemplates(context.Context) ([]platform.Template, error) {
	if err := f.record(methodListTemplates); err != nil {
		return nil, err
	}
	return slices.Clone(f.templates), nil
}

func (f *fakeAPI) DownloadTemplate(_ context.Context, productID, versionID string) ([]byte, error) {
	if err := f.record(methodDownloadTemplate); err != nil {
		return nil, err
	}
	f.downloads = append(f.downloads, download{product: productID, version: versionID})
	return f.archive, nil
}

var _ API = (*fakeAPI)(nil)

// newFakeAPI is a platform with two usable runtimes and a debug one that is never offered, a region and a test
// region that is never offered, one application and one nodejs template.
func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		runtimes: []platform.Runtime{
			{ID: "r1", Name: "nodejs18", Language: "nodejs"},
			{ID: "r2", Name: "nodejs22", Language: "nodejs", Comments: "latest"},
			{ID: "r3", Name: "debug-runtime", Language: "debug"},
		},
		regions: []platform.Region{
			{Name: "AWS Europe (Ireland)", Alias: "aws.euw1"},
			{Name: "Test region", Alias: "aws.test1"},
		},
		apps: []platform.Application{{ID: "a1", Name: "App One"}},
		templates: []platform.Template{
			{ID: "p1", Name: "Starter Project - Node.js", ProgrammingLanguage: "nodejs", ActiveVersionID: "v1"},
		},
	}
}

// tarEntry is one member of a test archive; a zero typeflag is a regular file and a zero mode is 0644.
type tarEntry struct {
	name     string
	body     string
	typeflag byte
	mode     int64
}

func entry(name, body string) tarEntry { return tarEntry{name: name, body: body} }

// tarGz returns the tar.gz archive of entries, the format a template is downloaded in.
func tarGz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag, mode := e.typeflag, e.mode
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{Name: e.name, Typeflag: typeflag, Mode: mode, Size: int64(len(e.body))}
		require.NoError(t, tw.WriteHeader(header))
		_, err := io.WriteString(tw, e.body)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// spinnerLog records the labels of the spinners a run starts.
type spinnerLog struct {
	labels  []string
	stopped int
}

func (l *spinnerLog) start(label string) func() {
	l.labels = append(l.labels, label)
	return func() { l.stopped++ }
}

// harness is one run of init over a fake platform and a scripted prompter. The project directory does not exist
// yet; init makes it.
type harness struct {
	t       *testing.T
	dir     string
	api     *fakeAPI
	prompt  *script
	spin    *spinnerLog
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	streams *term.IO
}

func newHarness(t *testing.T, asks ...ask) *harness {
	t.Helper()
	streams, stdout, stderr := term.Test("", false)
	return &harness{
		t:       t,
		dir:     filepath.Join(t.TempDir(), "my-project"),
		api:     newFakeAPI(),
		prompt:  newScript(t, asks...),
		spin:    &spinnerLog{},
		stdout:  stdout,
		stderr:  stderr,
		streams: streams,
	}
}

func (h *harness) deps() deps {
	return deps{IO: h.streams, Prompt: h.prompt, API: h.api, DefaultRegion: "aws.euw1", spin: h.spin.start}
}

// run runs init for the harness's directory.
func (h *harness) run() error {
	h.t.Helper()
	return run(h.t.Context(), h.deps(), h.dir)
}

func (h *harness) path(name string) string { return filepath.Join(h.dir, name) }
