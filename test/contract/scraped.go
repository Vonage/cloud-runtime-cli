//go:build !windows

package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scraped is a string that other tools read from the output.
type Scraped struct {
	Name    string `yaml:"name"`
	Stream  string `yaml:"stream"`
	Pattern string `yaml:"pattern"`
	re      *regexp.Regexp
}

// ParseScraped reads the entries of scraped.yaml.
func ParseScraped(data []byte) ([]Scraped, error) {
	var out []Scraped
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("scraped strings: %w", err)
	}
	seen := map[string]bool{}
	for i := range out {
		s := &out[i]
		if s.Stream == "" {
			s.Stream = "stdout"
		}
		switch {
		case s.Name == "":
			return nil, fmt.Errorf("scraped strings: entry %d has no name", i)
		case seen[s.Name]:
			return nil, fmt.Errorf("scraped strings: the name %q is used twice", s.Name)
		case !slices.Contains([]string{"stdout", "stderr", "any"}, s.Stream):
			return nil, fmt.Errorf("scraped strings: %s: stream %q is not stdout, stderr or any", s.Name, s.Stream)
		}
		seen[s.Name] = true
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return nil, fmt.Errorf("scraped strings: %s: %w", s.Name, err)
		}
		s.re = re
	}
	return out, nil
}

// LoadScraped reads scraped.yaml from path.
func LoadScraped(path string) ([]Scraped, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseScraped(data)
}

// Matches reports whether one line matches the pattern.
func (s Scraped) Matches(line string) bool { return s.re.MatchString(line) }

// Lines returns the lines of the entry's stream that match, in order.
func (s Scraped) Lines(stdout, stderr string) []string {
	var streams []string
	switch s.Stream {
	case "stdout":
		streams = []string{stdout}
	case "stderr":
		streams = []string{stderr}
	default:
		streams = []string{stdout, stderr}
	}
	var out []string
	for _, text := range streams {
		for _, line := range strings.Split(text, "\n") {
			if s.Matches(line) {
				out = append(out, line)
			}
		}
	}
	return out
}

// CheckScrapes compares, for every entry the scenario is not exempt from, the
// matching lines of the recorded output with those of the output under test.
// Nothing excuses a difference: allowed diffs do not apply.
func CheckScrapes(sc Scenario, scrapes []Scraped, base Transcript, got Result) []Failure {
	var out []Failure
	for _, s := range scrapes {
		if slices.Contains(sc.ScrapeExcept, s.Name) {
			continue
		}
		want, have := s.Lines(base.Stdout, base.Stderr), s.Lines(got.Stdout, got.Stderr)
		if !slices.Equal(want, have) {
			detail := "the lines of the scraped string " + s.Name + " changed\n" +
				diffDetail(strings.Join(want, "\n"), strings.Join(have, "\n"))
			out = append(out, Failure{Scenario: sc.ID, Field: "scrape/" + s.Name, Detail: detail})
		}
	}
	return out
}

// ScrapeExpectations returns the problems of the scrape and scrape-except
// names of a scenario: an unknown name, or a scrape name that matches nothing
// in the baseline output.
func ScrapeExpectations(sc Scenario, scrapes []Scraped, stdout, stderr string) []string {
	byName := map[string]Scraped{}
	for _, s := range scrapes {
		byName[s.Name] = s
	}
	var problems []string
	for _, name := range sc.ScrapeExcept {
		if _, ok := byName[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: scrape-except names %q, which scraped.yaml does not define", sc.ID, name))
		}
	}
	for _, name := range sc.Scrape {
		s, ok := byName[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: scrape names %q, which scraped.yaml does not define", sc.ID, name))
		case len(s.Lines(stdout, stderr)) == 0:
			problems = append(problems, fmt.Sprintf("%s: scrape %q matches no line of the baseline output", sc.ID, name))
		}
	}
	return problems
}
