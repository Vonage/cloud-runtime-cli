package fakeplatform

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// world is the state of a running server: the expanded fixtures as requests
// changed them, and the counters of overrides and log polls. It is only used
// with Server.mu held.
type world struct {
	f        Fixtures
	matchers []matcher
	hits     []int          // matches counted per override
	polls    map[string]int // log polls per instance id
	created  int            // applications created so far
}

func newWorld(f Fixtures) (world, error) {
	w := world{f: f, hits: make([]int, len(f.Overrides)), polls: map[string]int{}}
	for i, o := range f.Overrides {
		m, err := parseMatch(o.Match)
		if err != nil {
			return world{}, fmt.Errorf("overrides[%d]: %w", i, err)
		}
		w.matchers = append(w.matchers, m)
	}
	return w, nil
}

// locked runs fn on the world with the lock held.
func (s *Server) locked(fn func(w *world)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.world)
}

// caller is the account that requests come from: the credentials are checked
// against it.
func (w *world) caller() string { return w.f.Account.Key }

// owner returns the account that a record's account field names.
func (w *world) owner(account string) string {
	if account == "" {
		return w.f.Account.Key
	}
	return account
}

func (w *world) ownedByCaller(in *Instance) bool {
	return !in.Deleted && w.owner(in.ProjectAccount) == w.caller()
}

func (w *world) visible(in *Instance) bool {
	return w.ownedByCaller(in) || (!in.Deleted && in.PackageOwner == w.caller())
}

// instances returns the instances that keep accepts, newest first.
func (w *world) instances(keep func(*Instance) bool) []*Instance {
	var out []*Instance
	for i := range w.f.Instances {
		if keep(&w.f.Instances[i]) {
			out = append(out, &w.f.Instances[i])
		}
	}
	slices.SortStableFunc(out, func(a, b *Instance) int { return timeOf(b.UpdatedAt).Compare(timeOf(a.UpdatedAt)) })
	return out
}

// timeOf parses a timestamp that validate accepted; an empty one is the zero time.
func timeOf(v string) time.Time {
	t, err := parseTime(v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// instanceByID returns the first instance with the id (compared without
// case) that keep accepts, or nil.
func (w *world) instanceByID(id string, keep func(*Instance) bool) *Instance {
	for i := range w.f.Instances {
		if in := &w.f.Instances[i]; strings.EqualFold(in.ID, id) && keep(in) {
			return in
		}
	}
	return nil
}

// byNames returns the visible instances called name in a project called
// project: those of the caller's own project first, then those running a
// package of the caller, each newest first.
func (w *world) byNames(project, name string) []*Instance {
	named := func(in *Instance) bool { return in.ProjectName == project && in.Name == name }
	own := w.instances(func(in *Instance) bool { return named(in) && w.ownedByCaller(in) })
	packaged := w.instances(func(in *Instance) bool {
		return named(in) && w.visible(in) && !w.ownedByCaller(in)
	})
	return append(own, packaged...)
}

// projectID is the id of the instance's project, "" when it is not a fixture.
func (w *world) projectID(in *Instance) string {
	for _, p := range w.f.Projects {
		if p.Name == in.ProjectName && w.owner(p.Account) == w.owner(in.ProjectAccount) {
			return p.ID
		}
	}
	return ""
}

func (w *world) ownProject(match func(*Project) bool) (Project, bool) {
	for i := range w.f.Projects {
		if p := &w.f.Projects[i]; w.owner(p.Account) == w.caller() && match(p) {
			return *p, true
		}
	}
	return Project{}, false
}

// upsertInstance replaces the caller's live instance with the same project
// and name, as a redeploy does, or adds in.
func (w *world) upsertInstance(in Instance) {
	for i := range w.f.Instances {
		old := &w.f.Instances[i]
		if w.ownedByCaller(old) && old.ProjectName == in.ProjectName && old.Name == in.Name {
			*old = in
			return
		}
	}
	w.f.Instances = append(w.f.Instances, in)
}

// newestUpdate returns a time one minute after the newest instance update.
func (w *world) newestUpdate() string {
	newest := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	for _, in := range w.f.Instances {
		if t := timeOf(in.UpdatedAt); t.After(newest) {
			newest = t
		}
	}
	return newest.Add(time.Minute).UTC().Format(time.RFC3339)
}

// poll counts a poll of the instance's logs and returns its number.
func (w *world) poll(id string) int {
	id = strings.ToLower(id)
	w.polls[id]++
	return w.polls[id]
}

// numberedLine is a log line with its position in the fixtures, which makes its id.
type numberedLine struct {
	n    int
	line *LogLine
	at   time.Time
}

func (l numberedLine) id() string {
	return fmt.Sprintf("00000000-0000-4000-8000-0008%08d", l.n+1)
}

// logLines returns the instance's lines that poll number poll shows and that
// are later than after (when not nil), newest first, at most limit of them (0
// is no limit).
func (w *world) logLines(id string, poll int, after *time.Time, limit int) []numberedLine {
	var out []numberedLine
	for i := range w.f.Logs {
		l := &w.f.Logs[i]
		at := timeOf(l.Timestamp)
		if !strings.EqualFold(l.Instance, id) || poll <= l.VisibleAfterPolls || (after != nil && !at.After(*after)) {
			continue
		}
		out = append(out, numberedLine{n: i, line: l, at: at})
	}
	slices.SortStableFunc(out, func(a, b numberedLine) int { return b.at.Compare(a.at) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// without returns a new slice of the elements of list that drop rejects, and
// whether it dropped any.
func without[T any](list []T, drop func(T) bool) ([]T, bool) {
	out := make([]T, 0, len(list))
	for _, v := range list {
		if !drop(v) {
			out = append(out, v)
		}
	}
	return out, len(out) != len(list)
}
