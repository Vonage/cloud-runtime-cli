//go:build !windows

package contract

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var portPlaceholder = regexp.MustCompile(`\$\{PORT:([A-Za-z0-9_-]+)\}`)

// PortNames returns the distinct names of the ${PORT:<name>} placeholders in
// raw, in order of first appearance.
func PortNames(raw string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range portPlaceholder.FindAllStringSubmatch(raw, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// PortSet is a free port per name. A held port stays bound, on 127.0.0.1 and on
// every interface, until Close, so the process under test finds it in use.
type PortSet struct {
	ports map[string]int
	held  []net.Listener
}

// AllocatePorts picks a port for every name in names and hold; the ones in
// hold are bound and kept.
func AllocatePorts(names, hold []string) (*PortSet, error) {
	p := &PortSet{ports: map[string]int{}}
	for _, name := range hold {
		if _, ok := p.ports[name]; ok {
			continue
		}
		lns, port, err := listenEverywhere()
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("hold port %q: %w", name, err)
		}
		p.held = append(p.held, lns...)
		p.ports[name] = port
	}
	for _, name := range names {
		if _, ok := p.ports[name]; ok {
			continue
		}
		port, err := freePort()
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("port %q: %w", name, err)
		}
		p.ports[name] = port
	}
	return p, nil
}

// holdAttempts is how many ports listenEverywhere tries before it gives up.
const holdAttempts = 20

// listenEverywhere binds one free port on the loopback address and on every
// interface. A program that binds either of the two finds the port in use on
// every Unix; BSD kernels would otherwise let a wildcard bind and a loopback
// bind share a port.
func listenEverywhere() ([]net.Listener, int, error) {
	var lastErr error
	for attempt := 0; attempt < holdAttempts; attempt++ {
		loopback, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
		port := loopback.Addr().(*net.TCPAddr).Port
		wildcard, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			return []net.Listener{loopback, wildcard}, port, nil
		}
		_ = loopback.Close()
		lastErr = err
	}
	return nil, 0, lastErr
}

// Port is the port of a name, or 0 for an unknown name.
func (p *PortSet) Port(name string) int {
	if p == nil {
		return 0
	}
	return p.ports[name]
}

// Ports returns a copy of the name-to-port map.
func (p *PortSet) Ports() map[string]int {
	out := map[string]int{}
	if p != nil {
		for k, v := range p.ports {
			out[k] = v
		}
	}
	return out
}

// Close releases the held ports.
func (p *PortSet) Close() {
	for _, ln := range p.held {
		_ = ln.Close()
	}
	p.held = nil
}

// ExpandRunner replaces the placeholders that the runner owns in the raw
// scenario text. The ones for the fake platform (${FAKE} and the like) stay.
func ExpandRunner(raw string, l Layout, testapp string, p *PortSet) string {
	raw = strings.NewReplacer(
		"${SANDBOX}", l.Root, "${HOME}", l.Home, "${WORK}", l.Work, "${TMP}", l.Tmp,
		"${OUTSIDE}", l.Outside, "${BIN}", l.Bin, "${TESTAPP}", testapp,
	).Replace(raw)
	return portPlaceholder.ReplaceAllStringFunc(raw, func(m string) string {
		name := portPlaceholder.FindStringSubmatch(m)[1]
		if port := p.Port(name); port > 0 {
			return strconv.Itoa(port)
		}
		return m
	})
}
