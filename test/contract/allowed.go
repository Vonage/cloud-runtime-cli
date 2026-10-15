//go:build !windows

package contract

import (
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
)

// allowedFields are the fields an allowed diff can name.
var allowedFields = []string{"stdout", "stderr", "exit", "files", "requests"}

type allowRule struct{ glob, field string }

// AllowedDiffs are the documented differences from the baseline: a field of
// the scenarios that a glob matches is not compared.
type AllowedDiffs struct{ rules []allowRule }

// ParseAllowed reads the text of allowed-diffs.txt: lines of "<scenario-glob>
// <field> <reason>", where # starts a comment line. ids are the scenario ids
// a glob is checked against; a glob that matches none is an error.
func ParseAllowed(text string, ids []string) (AllowedDiffs, error) {
	var a AllowedDiffs
	var errs []error
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule, err := parseAllowedLine(line, ids)
		if err != nil {
			errs = append(errs, fmt.Errorf("allowed-diffs.txt line %d: %w", i+1, err))
			continue
		}
		a.rules = append(a.rules, rule)
	}
	return a, errors.Join(errs...)
}

func parseAllowedLine(line string, ids []string) (allowRule, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return allowRule{}, errors.New("a line is \"<scenario-glob> <field> <reason>\": the field is missing")
	}
	glob, field := fields[0], fields[1]
	if !slices.Contains(allowedFields, field) {
		return allowRule{}, fmt.Errorf("unknown field %q: use one of %s", field, strings.Join(allowedFields, ", "))
	}
	if len(fields) < 3 {
		return allowRule{}, errors.New("a line needs a reason after the field")
	}
	if _, err := path.Match(glob, ""); err != nil {
		return allowRule{}, fmt.Errorf("bad glob %q: %w", glob, err)
	}
	reason := strings.Join(fields[2:], " ")
	if ref := internalRefIn(reason); ref != "" {
		return allowRule{}, fmt.Errorf("the reason names the internal reference %q: give the reason in public words", ref)
	}
	if !slices.ContainsFunc(ids, func(id string) bool { ok, _ := path.Match(glob, id); return ok }) {
		return allowRule{}, fmt.Errorf("the glob %q matches no scenario", glob)
	}
	return allowRule{glob: glob, field: field}, nil
}

// LoadAllowed reads allowed-diffs.txt; a missing file allows nothing.
func LoadAllowed(file string, ids []string) (AllowedDiffs, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return AllowedDiffs{}, nil
	}
	if err != nil {
		return AllowedDiffs{}, err
	}
	return ParseAllowed(string(data), ids)
}

// Allows reports whether field of the scenario id is allowed to differ.
func (a AllowedDiffs) Allows(id, field string) bool {
	for _, r := range a.rules {
		if r.field != field {
			continue
		}
		if ok, _ := path.Match(r.glob, id); ok {
			return true
		}
	}
	return false
}
