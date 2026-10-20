package debug

import (
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dialTimeout = 5 * time.Second

// ipv6Available reports whether this machine can bind the IPv6 loopback address.
var ipv6Available = sync.OnceValue(func() bool {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
})

// requireIPv6 skips the test on a machine without the IPv6 loopback address.
func requireIPv6(t *testing.T) {
	t.Helper()
	if !ipv6Available() {
		t.Skip("the IPv6 loopback address is not available")
	}
}

// freePort returns a port that is free on 127.0.0.1 and, where IPv6 works, on [::1] too.
func freePort(t *testing.T) int {
	t.Helper()
	for range 50 {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		addr, ok := ln.Addr().(*net.TCPAddr)
		require.True(t, ok)
		require.NoError(t, ln.Close())
		if !ipv6Available() {
			return addr.Port
		}
		other, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(addr.Port)))
		if err == nil {
			_ = other.Close()
			return addr.Port
		}
	}
	t.Fatal("no port is free on both loopback addresses")
	return 0
}

// listenOn binds a test socket and closes it when the test ends.
func listenOn(t *testing.T, network, address string) net.Listener {
	t.Helper()
	ln, err := net.Listen(network, address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// lanAddress returns an IPv4 address of this machine that is not loopback or link-local, or "".
func lanAddress(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
			continue
		}
		return ipNet.IP.String()
	}
	return ""
}

// assertRefusesConnections checks that a dial to addr fails.
func assertRefusesConnections(t *testing.T, addr string) {
	t.Helper()
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(t.Context(), "tcp", addr)
	if err == nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "something answered on %s", addr)
}

// requireAccepts dials addr and checks that the listener accepted the connection.
func requireAccepts(t *testing.T, ln net.Listener, addr string) {
	t.Helper()
	accepted := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			err = conn.Close()
		}
		accepted <- err
	}()
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	select {
	case err := <-accepted:
		require.NoError(t, err)
	case <-time.After(dialTimeout):
		t.Fatalf("nothing was accepted on %s", addr)
	}
}

func closeAll(t *testing.T, listeners []net.Listener) {
	t.Helper()
	t.Cleanup(func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	})
}

// failIPv6With returns a listenFunc that fails binding the IPv6 loopback address with err and leaves IPv4 alone.
func failIPv6With(err error) listenFunc {
	return func(network, address string) (net.Listener, error) {
		if network == "tcp6" {
			return nil, err
		}
		return net.Listen(network, address)
	}
}

func ipv6Failure(call string, errno syscall.Errno) error {
	return &net.OpError{Op: "listen", Net: "tcp6", Err: os.NewSyscallError(call, errno)}
}

func TestListenLoopbackBindsBothFamilies(t *testing.T) {
	requireIPv6(t)
	port := freePort(t)

	listeners, err := listenLoopback(net.Listen, port)

	require.NoError(t, err)
	closeAll(t, listeners)
	require.Len(t, listeners, 2)
	requireAccepts(t, listeners[0], net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	requireAccepts(t, listeners[1], net.JoinHostPort("::1", strconv.Itoa(port)))
}

func TestListenLoopbackNeverBindsAllInterfaces(t *testing.T) {
	var asked []string
	record := func(network, address string) (net.Listener, error) {
		asked = append(asked, network+" "+address)
		return net.Listen(network, address)
	}
	port := freePort(t)

	listeners, err := listenLoopback(record, port)

	require.NoError(t, err)
	closeAll(t, listeners)
	text := strconv.Itoa(port)
	assert.Equal(t, []string{"tcp4 127.0.0.1:" + text, "tcp6 [::1]:" + text}, asked)
}

func TestListenLoopbackToleratesMissingIPv6(t *testing.T) {
	cases := map[string]error{
		"address family not supported":         ipv6Failure("socket", syscall.EAFNOSUPPORT),
		"address not available":                ipv6Failure("bind", syscall.EADDRNOTAVAIL),
		"windows address family not supported": ipv6Failure("socket", syscall.Errno(10047)),
		"windows address not available":        ipv6Failure("bind", syscall.Errno(10049)),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			listeners, err := listenLoopback(failIPv6With(failure), freePort(t))

			require.NoError(t, err)
			closeAll(t, listeners)
			require.Len(t, listeners, 1)
			assert.Equal(t, "127.0.0.1", listeners[0].Addr().(*net.TCPAddr).IP.String())
		})
	}
}

func TestListenLoopbackFailsOnAnyOtherIPv6Error(t *testing.T) {
	cases := map[string]error{
		"address in use":    ipv6Failure("bind", syscall.EADDRINUSE),
		"permission denied": ipv6Failure("bind", syscall.EACCES),
		"not a syscall":     errors.New("something else"),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			port := freePort(t)

			listeners, err := listenLoopback(failIPv6With(failure), port)

			require.Error(t, err)
			assert.Nil(t, listeners)
			// the IPv4 socket was closed again, so nothing is left bound
			listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		})
	}
}

func TestListenLoopbackFailsWhenIPv4PortBusy(t *testing.T) {
	port := freePort(t)
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))

	listeners, err := listenLoopback(net.Listen, port)

	require.Error(t, err)
	assert.Nil(t, listeners)
	assert.ErrorContains(t, err, strconv.Itoa(port))
}

func TestListenLoopbackFailsWhenIPv6PortBusy(t *testing.T) {
	requireIPv6(t)
	port := freePort(t)
	listenOn(t, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))

	listeners, err := listenLoopback(net.Listen, port)

	require.Error(t, err)
	assert.Nil(t, listeners)
	assert.ErrorContains(t, err, strconv.Itoa(port))
	// the IPv4 socket was closed again, so nothing is left bound
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}
