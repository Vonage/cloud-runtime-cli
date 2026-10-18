package debugsession

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

// readHeaderTimeout is how long a client may take to send its request headers.
const readHeaderTimeout = 10 * time.Second

var (
	// shutdownTimeout is how long running requests may take to finish once the session ends.
	shutdownTimeout = 5 * time.Second

	// listenTCP binds a socket; tests replace it to simulate bind errors.
	listenTCP = net.Listen
)

// Listeners are the local debugger's sockets on 127.0.0.1:<port> and, when IPv6 is available, [::1]:<port>.
type Listeners struct {
	V4 net.Listener
	V6 net.Listener // nil when IPv6 is unavailable
}

// Listen binds both listeners. Any IPv4 error is fatal; on IPv6 only "not available" errors are tolerated;
// EADDRINUSE on either is fatal. Called before the debug service is deployed.
func Listen(port int) (*Listeners, error) {
	v4, err := listenTCP("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, startError(err)
	}
	// For port 0 the system picks the port: IPv6 takes the one IPv4 got, so that both families share it.
	if bound := listenerPort(v4); bound != 0 {
		port = bound
	}
	v6, err := listenTCP("tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil {
		if !ipv6Unavailable(err) {
			// A process that holds the IPv6 port would otherwise get the SDK's traffic unnoticed.
			_ = v4.Close()
			return nil, startError(err)
		}
		v6 = nil
	}
	return &Listeners{V4: v4, V6: v6}, nil
}

func startError(err error) error {
	return fmt.Errorf("failed to start the local debugger: %w", err)
}

// ipv6Unavailable reports whether err only says that this machine has no IPv6 loopback address: the family is not
// supported or the address cannot be assigned, on Unix and with the Windows codes WSAEAFNOSUPPORT and WSAEADDRNOTAVAIL.
func ipv6Unavailable(err error) bool {
	return errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) ||
		errors.Is(err, syscall.Errno(10047)) || errors.Is(err, syscall.Errno(10049))
}

func (l *Listeners) Close() error {
	var first error
	for _, ln := range l.open() {
		// serve has usually closed them already.
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) && first == nil {
			first = err
		}
	}
	return first
}

// open returns the listeners that exist.
func (l *Listeners) open() []net.Listener {
	if l.V6 == nil {
		return []net.Listener{l.V4}
	}
	return []net.Listener{l.V4, l.V6}
}

// serve answers on every listener through LocalOnly until ctx ends, then lets running requests finish for a few
// seconds and closes what is left. A listener that fails ends the other one's server too, and serve returns that
// failure.
func (l *Listeners) serve(ctx context.Context, h http.Handler) error {
	srv := newDebuggerServer(ctx, listenerPort(l.V4), h)
	listeners := l.open()
	results := make(chan error, len(listeners))
	for _, ln := range listeners {
		go func() { results <- srv.Serve(ln) }()
	}

	var first error
	pending := len(listeners)
	select {
	case <-ctx.Done():
	case err := <-results:
		pending--
		first = serveFailure(err)
	}
	stopServer(srv)
	for ; pending > 0; pending-- {
		if err := serveFailure(<-results); first == nil {
			first = err
		}
	}
	return first
}

// newDebuggerServer is the server that answers on both listeners. Only the request headers have a time limit: a provider call
// can take minutes.
func newDebuggerServer(ctx context.Context, port int, h http.Handler) *http.Server {
	return &http.Server{
		Handler:           LocalOnly(port, h),
		ReadHeaderTimeout: readHeaderTimeout,
		// A request that is still waiting when the session ends is released with it.
		BaseContext: func(net.Listener) context.Context { return ctx },
		ErrorLog:    log.New(io.Discard, "", 0), // only internal/term writes to the terminal
	}
}

// serveFailure is nil for the error a server returns after it was stopped on purpose.
func serveFailure(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// stopServer waits for running requests and, when they take too long, ends their connections. The session is over
// either way, so the errors of closing the sockets say nothing that anyone could act on.
func stopServer(srv *http.Server) {
	// The session's context has ended already, so the wait gets one of its own.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = srv.Close()
}

// listenerPort is the port a listener is bound to. A listener of another kind has none, and then no Host passes the
// LocalOnly check.
func listenerPort(ln net.Listener) int {
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}
