//go:build !windows

package contract

import (
	"os"
	"sort"
	"strings"
)

const (
	fenceProxy   = "http://127.0.0.1:9"
	fenceNoProxy = "127.0.0.1,localhost"
)

// Leg is one of the two replays of a terminal scenario: the classic output that a person who opts out sees, and the
// presentation of an interactive terminal.
type Leg uint8

const (
	LegClassic Leg = iota
	LegRich
)

// leg is the leg a Config replays or records.
func (c Config) leg() Leg {
	if c.Rich {
		return LegRich
	}
	return LegClassic
}

// runsIn reports whether the scenario has a replay in the leg: the classic leg replays every scenario but a rich-only
// one, the rich leg the terminal scenarios that are not classic.
func (sc Scenario) runsIn(leg Leg) bool {
	if leg == LegRich {
		return sc.runsRich()
	}
	return sc.runsClassic()
}

// legEnv is what the harness adds for a leg. The classic leg of a scenario that runs in both presentations opts out of
// the new one; everything else is left to the scenario, which states its own environment.
func legEnv(sc Scenario, leg Leg) []string {
	if leg == LegClassic && sc.Presentation == presentationBoth {
		return []string{"VCR_UI=classic"}
	}
	return nil
}

// ChildEnv is the whole environment of a scenario's process, built from
// nothing but this list: the sandbox, the clock and colour settings, the
// terminal, what the leg adds and the network fence. Go never proxies
// loopback, so the fake stays reachable while any real host fails with
// "proxyconnect tcp: dial tcp 127.0.0.1:9: connect: connection refused". The
// scenario's own env and extra come last and win.
func ChildEnv(l Layout, sc Scenario, leg Leg, extra []string) []string {
	env := []string{
		"HOME=" + l.Home,
		"TMPDIR=" + l.Tmp,
		"XDG_CONFIG_HOME=" + l.Home + "/.config",
		"XDG_CACHE_HOME=" + l.Home + "/.cache",
		"PATH=" + os.Getenv("PATH"),
		"TZ=UTC",
		"NO_COLOR=1",
	}
	if sc.TTY {
		env = append(env, "TERM=xterm")
	}
	env = append(env, legEnv(sc, leg)...)
	env = append(env, "HTTPS_PROXY="+fenceProxy, "HTTP_PROXY="+fenceProxy, "NO_PROXY="+fenceNoProxy)
	keys := make([]string, 0, len(sc.Env))
	for k := range sc.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+sc.Env[k])
	}
	return append(env, extra...)
}

// envValue is the value a process gets for key from a list of NAME=value entries: the last entry wins, and a
// variable that is not there is empty.
func envValue(env []string, key string) string {
	value := ""
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			value = v
		}
	}
	return value
}
