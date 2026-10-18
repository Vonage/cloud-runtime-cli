//go:build !windows

package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultTimeout = 60 * time.Second
	configNone     = "none"
	fileTypeDir    = "dir"
	fileTypeFifo   = "fifo"

	// The presentations a scenario can be replayed in: a terminal scenario runs in both by default, a scenario
	// without a terminal only has the classic output.
	presentationBoth    = "both"
	presentationClassic = "classic"
	presentationRich    = "rich"
)

// gateKeys are the variables that decide the presentation of a terminal. A scenario that runs in both presentations
// leaves them to the harness.
var gateKeys = []string{"VCR_UI", "CI", "CLICOLOR_FORCE", "TERM"}

// groups are the groups a scenario can belong to.
var groups = []string{
	"version", "help", "completion", "root", "config", "bootstrap", "app",
	"secret", "instance", "deploy", "init", "debug", "configure", "upgrade",
}

// readsRequired are the groups whose scenarios must list their expect-reads.
var readsRequired = []string{"root", "config", "bootstrap", "app", "secret", "instance", "deploy", "init", "debug", "configure"}

var (
	scenarioIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	expectReadPattern   = regexp.MustCompile(`^[a-z0-9]+ GET /\S*$`)
	holdPortPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	validRequirements   = []string{"non-root", "ipv6", "ssh-keygen"}
	internalRefPatterns = compileInternalRefPatterns()
	validFileTypes      = []string{fileTypeDir, fileTypeFifo}
	errDescriptionShape = errors.New("description is required and must be one line")
)

// compileInternalRefPatterns covers the references to internal work that must
// not appear in a public description: ticket keys, short numbered identifiers
// and row references.
func compileInternalRefPatterns() []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range []string{
		`\b[A-Z][A-Z0-9]+-[0-9]+\b`, `\bT[0-9]{2}\b`, `\b[A-Z]{1,3}\.[A-Z][0-9]+\b`, `\bD[0-9]+[a-f]?\b`,
		`\bN[0-9]+\b`, `\bH[0-9]\b`, `\bC-?[0-9]{1,2}\b`, `\brow [0-9]+\b`,
	} {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

// internalRefIn returns the first internal reference found in s, or "".
func internalRefIn(s string) string {
	for _, re := range internalRefPatterns {
		if m := re.FindString(s); m != "" {
			return m
		}
	}
	return ""
}

// FileSpec is a file the sandbox gets before the run; exactly one of Content,
// From, Link and Type is set.
type FileSpec struct {
	Path    string  `yaml:"path"`
	Content *string `yaml:"content"`
	Mode    *int    `yaml:"mode"`
	From    string  `yaml:"from"`
	Link    string  `yaml:"link"`
	Type    string  `yaml:"type"`
	Mtime   string  `yaml:"mtime"`
}

// Scenario is one command run, as described by a scenarios/<id>.yaml file.
type Scenario struct {
	ID                 string
	Raw                []byte // the text it was parsed from, before any placeholder was expanded
	Description        string
	Group              string
	Args               []string
	Cwd                string
	TTY                bool
	Stdin              *string
	Build              string
	Exe                string
	Requires           []string
	RequiresHotfix     bool
	HoldPorts          []string
	Env                map[string]string
	Presentation       string // both, classic or rich
	Config             string
	ConfigFile         string
	Files              []FileSpec
	Fixtures           yaml.Node
	Steps              []Step
	ExpectReads        []string
	Scrape             []string
	ScrapeExcept       []string
	FencedHost         string
	FencedHostBaseline string
	Timeout            time.Duration
}

// rawScenario is the file as written: presence matters for args and
// expect-reads, and the timeout is text.
type rawScenario struct {
	Description        string            `yaml:"description"`
	Group              string            `yaml:"group"`
	Args               *[]string         `yaml:"args"`
	Cwd                string            `yaml:"cwd"`
	TTY                bool              `yaml:"tty"`
	Stdin              *string           `yaml:"stdin"`
	Build              string            `yaml:"build"`
	Exe                string            `yaml:"exe"`
	Requires           []string          `yaml:"requires"`
	RequiresHotfix     bool              `yaml:"requires-hotfix"`
	HoldPorts          []string          `yaml:"hold-ports"`
	Env                map[string]string `yaml:"env"`
	Presentation       string            `yaml:"presentation"`
	Config             string            `yaml:"config"`
	ConfigFile         string            `yaml:"config-file"`
	Files              []FileSpec        `yaml:"files"`
	Fixtures           yaml.Node         `yaml:"fixtures"`
	Steps              []rawStep         `yaml:"steps"`
	ExpectReads        *[]string         `yaml:"expect-reads"`
	Scrape             []string          `yaml:"scrape"`
	ScrapeExcept       []string          `yaml:"scrape-except"`
	FencedHost         string            `yaml:"fenced-host"`
	FencedHostBaseline string            `yaml:"fenced-host-baseline"`
	Timeout            string            `yaml:"timeout"`
}

// ParseScenario decodes and checks a scenario file; unknown keys are errors.
func ParseScenario(id string, raw []byte) (Scenario, error) {
	sc, err := parseScenario(id, raw)
	if err != nil {
		return Scenario{}, fmt.Errorf("scenario %s: %w", id, err)
	}
	return sc, nil
}

func parseScenario(id string, raw []byte) (Scenario, error) {
	if !scenarioIDPattern.MatchString(id) {
		return Scenario{}, fmt.Errorf("the id %q must be lower case letters, digits and dashes", id)
	}
	var r rawScenario
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil && !errors.Is(err, io.EOF) {
		return Scenario{}, err
	}
	sc := Scenario{
		ID: id, Raw: slices.Clone(raw), Description: r.Description, TTY: r.TTY, Stdin: r.Stdin,
		Requires: r.Requires, RequiresHotfix: r.RequiresHotfix, HoldPorts: r.HoldPorts, Env: r.Env,
		ConfigFile: r.ConfigFile, Files: r.Files, Fixtures: r.Fixtures, Scrape: r.Scrape, ScrapeExcept: r.ScrapeExcept,
		FencedHost: r.FencedHost, FencedHostBaseline: r.FencedHostBaseline,
		Cwd: orDefault(r.Cwd, "work"), Build: orDefault(r.Build, "release"),
		Exe: orDefault(r.Exe, "stamped"), Config: orDefault(r.Config, "default"),
		Presentation: orDefault(r.Presentation, defaultPresentation(r.TTY)),
	}
	if r.Args == nil {
		return Scenario{}, errors.New("args is required (it may be [])")
	}
	sc.Args = *r.Args
	if r.ExpectReads != nil {
		sc.ExpectReads = *r.ExpectReads
	}
	var err error
	if sc.Group, err = resolveGroup(id, r.Group); err != nil {
		return Scenario{}, err
	}
	if sc.Timeout, err = parseDuration("timeout", r.Timeout); err != nil {
		return Scenario{}, err
	}
	if sc.Timeout == 0 {
		sc.Timeout = defaultTimeout
	}
	for _, rs := range r.Steps {
		step, err := parseStep(rs)
		if err != nil {
			return Scenario{}, err
		}
		sc.Steps = append(sc.Steps, step)
	}
	return sc, errors.Join(validateScenario(sc, r.ExpectReads != nil), validateFiles(sc.Files))
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// resolveGroup is the group key, else the text of the id before the first dash.
func resolveGroup(id, key string) (string, error) {
	g := key
	if g == "" {
		g, _, _ = strings.Cut(id, "-")
	}
	if !slices.Contains(groups, g) {
		if key == "" {
			return "", fmt.Errorf("the id does not start with a group name and has no group: key; groups are %s", strings.Join(groups, " "))
		}
		return "", fmt.Errorf("unknown group %q; groups are %s", key, strings.Join(groups, " "))
	}
	return g, nil
}

// validateScenario checks the values that do not need the files.
func validateScenario(sc Scenario, hasReads bool) error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if sc.Description == "" || strings.Contains(sc.Description, "\n") {
		errs = append(errs, errDescriptionShape)
	} else if ref := internalRefIn(sc.Description); ref != "" {
		add("description names the internal reference %q: describe the behaviour in public words", ref)
	}
	if sc.TTY && sc.Stdin != nil {
		add("tty: true excludes stdin")
	}
	if sc.FencedHost != "" && len(sc.ExpectReads) > 0 {
		add("fenced-host needs expect-reads: []")
	}
	if sc.FencedHostBaseline != "" && sc.FencedHost == "" {
		add("fenced-host-baseline needs fenced-host")
	}
	if !hasReads && slices.Contains(readsRequired, sc.Group) {
		add("expect-reads is required in group %s (it may be [])", sc.Group)
	}
	for _, e := range sc.ExpectReads {
		if !expectReadPattern.MatchString(e) {
			add(`expect-reads entry %q must look like "euw1 GET /regions"`, e)
		}
	}
	errs = append(errs, validateChoices(sc)...)
	errs = append(errs, validatePresentation(sc)...)
	return errors.Join(errs...)
}

// defaultPresentation is both for a terminal scenario and classic for one without a terminal.
func defaultPresentation(tty bool) string {
	if tty {
		return presentationBoth
	}
	return presentationClassic
}

// validatePresentation checks the presentation against the rest of the scenario: both and rich need a terminal, and
// both leaves the variables of the gate to the harness.
func validatePresentation(sc Scenario) []error {
	var errs []error
	switch sc.Presentation {
	case presentationClassic:
	case presentationBoth, presentationRich:
		if !sc.TTY {
			errs = append(errs, fmt.Errorf("presentation %s needs tty: true", sc.Presentation))
		}
		if sc.Presentation == presentationBoth && slices.ContainsFunc(gateKeys, func(k string) bool { _, ok := sc.Env[k]; return ok }) {
			errs = append(errs, errors.New("presentation both owns VCR_UI, CI, CLICOLOR_FORCE and TERM: use classic or rich"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown presentation %q", sc.Presentation))
	}
	return errs
}

// runsClassic reports whether make contract replays the scenario: every scenario but a rich-only one.
func (sc Scenario) runsClassic() bool { return sc.Presentation != presentationRich }

// runsRich reports whether make contract-rich replays the scenario: a terminal scenario that is not classic.
func (sc Scenario) runsRich() bool {
	return sc.Presentation == presentationBoth || sc.Presentation == presentationRich
}

func validateChoices(sc Scenario) []error {
	var errs []error
	oneOf := func(key, v string, allowed ...string) {
		if !slices.Contains(allowed, v) {
			errs = append(errs, fmt.Errorf("%s: %q is not one of %s", key, v, strings.Join(allowed, ", ")))
		}
	}
	oneOf("build", sc.Build, "release", "dev")
	oneOf("exe", sc.Exe, "stamped", "copy")
	oneOf("config", sc.Config, "default", configNone)
	for _, r := range sc.Requires {
		oneOf("requires", r, validRequirements...)
	}
	for _, p := range sc.HoldPorts {
		if !holdPortPattern.MatchString(p) {
			errs = append(errs, fmt.Errorf("hold-ports: %q is not a port name", p))
		}
	}
	if sc.Config == configNone && sc.ConfigFile != "" {
		errs = append(errs, errors.New("config-file cannot be combined with config: none"))
	}
	return errs
}

// validateFiles checks every files entry: a relative path inside the sandbox
// and exactly one way of making it.
func validateFiles(files []FileSpec) error {
	var errs []error
	for i, f := range files {
		if f.Path == "" || path.IsAbs(f.Path) || filepath.IsAbs(f.Path) || path.Clean(f.Path) == ".." || strings.HasPrefix(path.Clean(f.Path), "../") {
			errs = append(errs, fmt.Errorf("files[%d]: path %q must be relative and inside the sandbox", i, f.Path))
		}
		kinds := 0
		for _, set := range []bool{f.Content != nil, f.From != "", f.Link != "", f.Type != ""} {
			if set {
				kinds++
			}
		}
		if kinds != 1 {
			errs = append(errs, fmt.Errorf("files[%d] (%s): exactly one of content, from, link and type is needed", i, f.Path))
		}
		if f.Type != "" && !slices.Contains(validFileTypes, f.Type) {
			errs = append(errs, fmt.Errorf("files[%d]: type %q is not dir or fifo", i, f.Type))
		}
		if f.Mtime != "" {
			if _, err := time.Parse(time.RFC3339, f.Mtime); err != nil {
				errs = append(errs, fmt.Errorf("files[%d]: mtime %q is not an RFC 3339 time", i, f.Mtime))
			}
		}
	}
	return errors.Join(errs...)
}

// LoadScenarios reads every *.yaml file of dir, in id order. A missing
// folder holds no scenarios.
func LoadScenarios(dir string) ([]Scenario, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]Scenario, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		sc, err := ParseScenario(strings.TrimSuffix(name, ".yaml"), raw)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// Reparse parses the scenario again from its raw text with expand applied to
// it, which is how placeholders are replaced before the text is decoded.
func (sc Scenario) Reparse(expand func(string) string) (Scenario, error) {
	return ParseScenario(sc.ID, []byte(expand(string(sc.Raw))))
}
