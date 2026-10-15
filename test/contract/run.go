//go:build !windows

package contract

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// Config tells the replayer and the recorder where everything is. The
// env-driven tests build it from the variables of the Makefile; the unit tests
// point it at temporary folders and a stand-in binary.
type Config struct {
	RepoRoot         string
	ScenariosDir     string
	TranscriptsDir   string
	AllowedDiffsFile string
	ScrapedFile      string
	Groups           map[string]bool // the groups to run; empty means all
	Filter           string          // a glob over scenario ids; empty means all
	Bin              string          // the binary to run; DevBin the one that has no version stamp
	DevBin           string
	NoGraphQL        bool   // the binary is the one under test: it must not send GraphQL and its reads are checked
	Port             int    // the one port the fake binds; 0 picks a free one per run
	BaselineLabel    string // "<ref> <sha>", the header of the transcripts a recording writes
	ExtraEnv         []string
}

func (c Config) contractDir() string { return filepath.Join(c.RepoRoot, "test", "contract") }

// Result is what one run produced: the exit and the four sections as stored in
// a transcript (normalised and escaped), and the raw request log.
type Result struct {
	Exit, Stdout, Stderr, Files, Requests string
	Log                                   []fakeplatform.Request
}

// scopedTB runs the cleanups that the fake registers at the end of a run
// rather than at the end of the test, so that a scenario can run twice.
type scopedTB struct {
	testing.TB
	cleanups []func()
}

func (s *scopedTB) Cleanup(f func()) { s.cleanups = append(s.cleanups, f) }

func (s *scopedTB) release() {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
	s.cleanups = nil
}

// run is a scenario that is set up: its sandbox, ports and fake platform.
type run struct {
	sc      Scenario // fully expanded
	l       Layout
	srv     *fakeplatform.Server
	ports   *PortSet
	testapp string
	port    int
	cdir    string
}

// RunScenario runs one scenario with bin in a sandbox of its own against a
// fresh fake platform. The error is a problem of the run itself (a step that
// timed out, a process that would not start); a command that fails is a
// Result with its exit.
func RunScenario(t testing.TB, bin string, sc Scenario, c Config) (Result, error) {
	t.Helper()
	if sc.TTY {
		t.Skip("terminal scenarios are driven by the terminal runner")
	}
	for _, req := range sc.Requires {
		if why := RequirementUnmet(req); why != "" {
			t.Skipf("%s: %s", sc.ID, why)
		}
	}
	scope := &scopedTB{TB: t}
	defer scope.release()
	r, err := prepare(scope, sc, c)
	if err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", sc.ID, err)
	}
	defer r.ports.Close()
	res, err := r.execute(bin, c)
	if err != nil {
		err = fmt.Errorf("scenario %s: %w", sc.ID, err)
	}
	return res, err
}

// prepare creates the sandbox, allocates the ports, expands the runner's
// placeholders, starts the fake on the run's port and expands the fake's.
func prepare(t testing.TB, sc Scenario, c Config) (*run, error) {
	r := &run{cdir: c.contractDir(), port: c.Port}
	var err error
	if r.l, err = CreateSandbox(t.TempDir()); err != nil {
		return nil, err
	}
	if r.port == 0 {
		if r.port, err = freePort(); err != nil {
			return nil, err
		}
	}
	raw := string(sc.Raw)
	if strings.Contains(raw, "${TESTAPP}") {
		if r.testapp, err = testappBinary(t, c); err != nil {
			return nil, err
		}
	}
	if r.ports, err = AllocatePorts(PortNames(raw), sc.HoldPorts); err != nil {
		return nil, err
	}
	pass1, err := sc.Reparse(func(s string) string { return ExpandRunner(s, r.l, r.testapp, r.ports) })
	if err != nil {
		r.ports.Close()
		return nil, err
	}
	fx, err := decodeFixtures(pass1.Fixtures, r.cdir)
	if err != nil {
		r.ports.Close()
		return nil, err
	}
	r.srv = fakeplatform.StartOn(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(r.port)), fx)
	if r.sc, err = pass1.Reparse(r.srv.Expand); err != nil {
		r.ports.Close()
		return nil, err
	}
	return r, nil
}

// decodeFixtures decodes the fixtures of a scenario over the defaults; a
// relative archiveFrom is resolved against the contract folder first.
func decodeFixtures(node yaml.Node, dir string) (fakeplatform.Fixtures, error) {
	if node.Kind == 0 {
		return fakeplatform.Defaults(), nil
	}
	resolveArchiveFrom(&node, dir)
	data, err := yaml.Marshal(&node)
	if err != nil {
		return fakeplatform.Fixtures{}, err
	}
	return fakeplatform.Decode(data)
}

func resolveArchiveFrom(n *yaml.Node, dir string) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Value == "archiveFrom" && v.Kind == yaml.ScalarNode && v.Value != "" && !filepath.IsAbs(v.Value) {
				v.Value = filepath.Join(dir, filepath.FromSlash(v.Value))
			}
		}
	}
	for _, c := range n.Content {
		resolveArchiveFrom(c, dir)
	}
}

// executable is the binary to run: the dev build when the scenario asks for
// one, copied to bin/vcr in the sandbox when it asks for a copy.
func (r *run) executable(bin string, c Config) (string, error) {
	exe := bin
	if r.sc.Build == "dev" && c.DevBin != "" {
		exe = c.DevBin
	}
	if r.sc.Exe != "copy" {
		return exe, nil
	}
	dst := filepath.Join(r.l.Bin, "vcr")
	return dst, copyExecutable(exe, dst)
}

func (r *run) execute(bin string, c Config) (Result, error) {
	if err := r.l.WriteConfig(r.sc, r.srv.Expand); err != nil {
		return Result{}, err
	}
	expand := func(s string) string { return r.srv.Expand(ExpandRunner(s, r.l, r.testapp, r.ports)) }
	if err := r.l.Materialise(r.sc.Files, r.cdir, expand); err != nil {
		return Result{}, err
	}
	exe, err := r.executable(bin, c)
	if err != nil {
		return Result{}, err
	}
	before, err := TakeSnapshot(r.l.Root)
	if err != nil {
		return Result{}, err
	}
	p, err := startProc(exe, r.sc, r.l, ChildEnv(r.l, r.sc, c.ExtraEnv))
	if err != nil {
		return Result{}, err
	}
	d := newDriver(r.sc, p, r.srv)
	runErr := d.run()
	p.stop()
	after, err := TakeSnapshot(r.l.Root)
	if err != nil {
		return Result{}, err
	}
	return r.result(p, d, before, after), runErr
}

// result normalises everything the run produced.
func (r *run) result(p *proc, d *driver, before, after Snapshot) Result {
	otherPort := 0
	if u, err := url.Parse(r.srv.OtherURL()); err == nil {
		otherPort, _ = strconv.Atoi(u.Port())
	}
	n := NewNormaliser(r.l, r.testapp, r.port, otherPort, r.ports.Ports(), ProtectedLiterals(r.sc, r.cdir))
	log := r.srv.Requests()
	obs := Observed{Requests: log, PeerEvents: r.srv.Peer().Events(), Local: d.local, Public: d.public}
	return Result{
		Exit:     p.exitLine(),
		Stdout:   Escape(n.Stream(p.out.String(), false)),
		Stderr:   Escape(n.Stream(p.err.String(), true)),
		Files:    Escape(RenderFiles(DiffSnapshots(before, after, n))),
		Requests: Escape(RenderRequests(obs, n)),
		Log:      log,
	}
}
