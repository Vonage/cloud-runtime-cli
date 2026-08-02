package logs

import (
	"fmt"
	"regexp"
	"strings"
)

// Level is the severity ladder used for threshold filtering.
type Level int

const (
	LevelNone  Level = 0 // no threshold set
	LevelTrace Level = 1
	LevelDebug Level = 2
	LevelInfo  Level = 3
	LevelWarn  Level = 4
	LevelError Level = 5
	LevelFatal Level = 6
)

var levelNames = map[string]Level{
	"trace": LevelTrace,
	"debug": LevelDebug,
	"info":  LevelInfo,
	"warn":  LevelWarn,
	"error": LevelError,
	"fatal": LevelFatal,
}

var levelOrder = []string{"trace", "debug", "info", "warn", "error", "fatal"}

// ParseLevel maps a level name to a Level. ok is false for unknown names.
func ParseLevel(name string) (Level, bool) {
	l, ok := levelNames[strings.ToLower(strings.TrimSpace(name))]
	return l, ok
}

// String returns the lower-case level name, or "" for LevelNone.
func (l Level) String() string {
	for name, v := range levelNames {
		if v == l {
			return name
		}
	}
	return ""
}

// Filter decides which entries are shown. The zero value matches everything.
type Filter struct {
	MinLevel   Level
	Include    *regexp.Regexp
	Exclude    *regexp.Regexp
	SourceType string
	// Replicas is keyed by replica short id (r1, r2, ...). An empty map means
	// all replicas are visible.
	Replicas map[string]bool

	includeSrc string
	excludeSrc string
}

// SetInclude compiles and installs the include pattern; "" clears it.
func (f *Filter) SetInclude(pattern string) error {
	if pattern == "" {
		f.Include, f.includeSrc = nil, ""
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid include pattern %q: %w", pattern, err)
	}
	f.Include, f.includeSrc = re, pattern
	return nil
}

// SetExclude compiles and installs the exclude pattern; "" clears it.
func (f *Filter) SetExclude(pattern string) error {
	if pattern == "" {
		f.Exclude, f.excludeSrc = nil, ""
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
	}
	f.Exclude, f.excludeSrc = re, pattern
	return nil
}

// Match reports whether the entry passes every active filter.
func (f *Filter) Match(e Entry) bool {
	if f.MinLevel != LevelNone {
		lvl, ok := ParseLevel(e.Level)
		if !ok || lvl < f.MinLevel {
			return false
		}
	}
	if f.SourceType != "" && f.SourceType != e.SourceType {
		return false
	}
	if f.Include != nil && !f.Include.MatchString(e.Message) {
		return false
	}
	if f.Exclude != nil && f.Exclude.MatchString(e.Message) {
		return false
	}
	if len(f.Replicas) > 0 && e.ReplicaID != "" && !f.Replicas[e.ReplicaID] {
		return false
	}
	return true
}

// Summary renders the active filters for the status footer.
func (f *Filter) Summary() string {
	var parts []string
	if f.MinLevel != LevelNone {
		parts = append(parts, "level>="+f.MinLevel.String())
	}
	if f.SourceType != "" {
		parts = append(parts, "source="+f.SourceType)
	}
	if f.includeSrc != "" {
		parts = append(parts, "/"+f.includeSrc+"/")
	}
	if f.excludeSrc != "" {
		parts = append(parts, "!/"+f.excludeSrc+"/")
	}
	if n := len(f.Replicas); n > 0 {
		parts = append(parts, fmt.Sprintf("%d replica(s)", n))
	}
	if len(parts) == 0 {
		return "no filters"
	}
	return strings.Join(parts, " · ")
}

// NextLevel cycles the threshold: none -> trace -> ... -> fatal -> none.
// Used by the Phase 2 interactive key handler.
func NextLevel(l Level) Level {
	if l == LevelNone {
		return LevelTrace
	}
	if l >= LevelFatal {
		return LevelNone
	}
	return l + 1
}

// LevelNames returns a copy of the ladder in order, for help text. It copies so
// a caller cannot reorder or overwrite the shared package slice for everyone
// else in the process.
func LevelNames() []string {
	names := make([]string, len(levelOrder))
	copy(names, levelOrder)
	return names
}
