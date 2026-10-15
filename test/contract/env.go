//go:build !windows

package contract

import (
	"os"
	"sort"
)

const (
	fenceProxy   = "http://127.0.0.1:9"
	fenceNoProxy = "127.0.0.1,localhost"
)

// ChildEnv is the whole environment of a scenario's process, built from
// nothing but this list: the sandbox, the clock and colour settings, and the
// network fence. Go never proxies loopback, so the fake stays reachable while
// any real host fails with "proxyconnect tcp: dial tcp 127.0.0.1:9: connect:
// connection refused". The scenario's own env and extra come last and win.
func ChildEnv(l Layout, sc Scenario, extra []string) []string {
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
