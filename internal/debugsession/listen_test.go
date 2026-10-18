package debugsession

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubListen replaces the socket call for the rest of the test.
func stubListen(t *testing.T, stub func(network, address string) (net.Listener, error)) {
	t.Helper()
	original := listenTCP
	listenTCP = stub
	t.Cleanup(func() { listenTCP = original })
}

// failIPv6With makes binding the IPv6 loopback address fail with err and leaves IPv4 alone.
func failIPv6With(t *testing.T, err error) {
	t.Helper()
	original := listenTCP
	stubListen(t, func(network, address string) (net.Listener, error) {
		if network == "tcp6" {
			return nil, err
		}
		return original(network, address)
	})
}

func ipv6Failure(call string, errno syscall.Errno) error {
	return &net.OpError{Op: "listen", Net: "tcp6", Err: os.NewSyscallError(call, errno)}
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
	conn, err := testDialer.DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	select {
	case err := <-accepted:
		require.NoError(t, err)
	case <-time.After(waitLimit):
		t.Fatalf("nothing was accepted on %s", addr)
	}
}

// assertNothingListening checks that a dial to addr is refused. Unlike a blocking Accept on a listener that is still
// open, it fails at once when the socket was not closed.
func assertNothingListening(t *testing.T, addr string) {
	t.Helper()
	conn, err := testDialer.DialContext(t.Context(), "tcp", addr)
	if err == nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "something still accepts connections on %s", addr)
}

func TestListenBindsBothFamilies(t *testing.T) {
	port := freePort(t)
	l, err := Listen(port)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	require.NotNil(t, l.V4)
	requireAccepts(t, l.V4, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))

	requireIPv6(t)
	require.NotNil(t, l.V6)
	requireAccepts(t, l.V6, net.JoinHostPort("::1", strconv.Itoa(port)))
}

func TestListenWithPortZeroSharesOnePortAcrossFamilies(t *testing.T) {
	requireIPv6(t)

	l, err := Listen(0)

	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	require.NotNil(t, l.V6)
	assert.NotZero(t, listenerPort(l.V4))
	assert.Equal(t, listenerPort(l.V4), listenerPort(l.V6))
}

func TestListenNeverBindsAllInterfaces(t *testing.T) {
	var asked []string
	original := listenTCP
	stubListen(t, func(network, address string) (net.Listener, error) {
		asked = append(asked, network+" "+address)
		return original(network, address)
	})
	port := freePort(t)

	l, err := Listen(port)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	text := strconv.Itoa(port)
	assert.Equal(t, []string{"tcp4 127.0.0.1:" + text, "tcp6 [::1]:" + text}, asked)
}

func TestListenToleratesMissingIPv6(t *testing.T) {
	cases := map[string]error{
		"address family not supported":         ipv6Failure("socket", syscall.EAFNOSUPPORT),
		"address not available":                ipv6Failure("bind", syscall.EADDRNOTAVAIL),
		"windows address family not supported": ipv6Failure("socket", syscall.Errno(10047)),
		"windows address not available":        ipv6Failure("bind", syscall.Errno(10049)),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			failIPv6With(t, failure)

			l, err := Listen(freePort(t))

			require.NoError(t, err)
			t.Cleanup(func() { _ = l.Close() })
			assert.NotNil(t, l.V4)
			assert.Nil(t, l.V6)
		})
	}
}

func TestListenFailsOnAnyOtherIPv6Error(t *testing.T) {
	cases := map[string]error{
		"address in use":    ipv6Failure("bind", syscall.EADDRINUSE),
		"permission denied": ipv6Failure("bind", syscall.EACCES),
		"not a syscall":     errors.New("something else"),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			failIPv6With(t, failure)
			port := freePort(t)

			l, err := Listen(port)

			require.Error(t, err)
			assert.Nil(t, l)
			assert.ErrorContains(t, err, "failed to start the local debugger")
			// the IPv4 socket was closed again, so nothing is left bound
			listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		})
	}
}

func TestListenFailsWhenIPv4PortBusy(t *testing.T) {
	port := freePort(t)
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))

	l, err := Listen(port)

	require.Error(t, err)
	assert.Nil(t, l)
	assert.ErrorContains(t, err, "failed to start the local debugger")
	assert.ErrorContains(t, err, strconv.Itoa(port))
	if ipv6Available() {
		// the IPv6 side was never bound
		listenOn(t, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	}
}

func TestListenFailsWhenIPv6PortBusy(t *testing.T) {
	requireIPv6(t)
	port := freePort(t)
	listenOn(t, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))

	l, err := Listen(port)

	require.Error(t, err)
	assert.Nil(t, l)
	assert.ErrorContains(t, err, "failed to start the local debugger")
	assert.ErrorContains(t, err, strconv.Itoa(port))
	// the IPv4 socket was closed again, so nothing is left bound
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
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

func TestListenerNotReachableOnLANAddress(t *testing.T) {
	lan := lanAddress(t)
	if lan == "" {
		t.Skip("this machine has no address other than loopback")
	}
	port := freePort(t)
	l, err := Listen(port)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(t.Context(), "tcp", net.JoinHostPort(lan, strconv.Itoa(port)))

	if err == nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "the debugger answered on %s", lan)
}

func TestCloseIsIdempotent(t *testing.T) {
	l, err := Listen(freePort(t))
	require.NoError(t, err)

	assert.NoError(t, l.Close())
	assert.NoError(t, l.Close())
}

func TestCloseWithoutIPv6Listener(t *testing.T) {
	l := &Listeners{V4: listenOn(t, "tcp4", "127.0.0.1:0")}

	assert.NoError(t, l.Close())
	assert.NoError(t, l.Close())
}

func TestCloseAfterTheListenersWereClosed(t *testing.T) {
	l, err := Listen(freePort(t))
	require.NoError(t, err)
	require.NoError(t, l.V4.Close())
	if l.V6 != nil {
		require.NoError(t, l.V6.Close())
	}

	assert.NoError(t, l.Close())
}

// failingListener closes the real listener, then reports err.
type failingListener struct {
	net.Listener
	err error
}

func (f failingListener) Close() error {
	_ = f.Listener.Close()
	return f.err
}

func TestCloseClosesBothAndReturnsFirstRealError(t *testing.T) {
	first := errors.New("first failure")
	v4 := listenOn(t, "tcp4", "127.0.0.1:0")
	v6 := listenOn(t, "tcp4", "127.0.0.1:0")
	addrs := []string{v4.Addr().String(), v6.Addr().String()}
	l := &Listeners{
		V4: failingListener{Listener: v4, err: first},
		V6: failingListener{Listener: v6, err: errors.New("second failure")},
	}

	err := l.Close()

	assert.Equal(t, first, err)
	for _, addr := range addrs {
		assertNothingListening(t, addr)
	}
}

func TestCloseSkipsAlreadyClosedErrors(t *testing.T) {
	second := errors.New("second failure")
	l := &Listeners{
		V4: failingListener{Listener: listenOn(t, "tcp4", "127.0.0.1:0"), err: fmt.Errorf("close: %w", net.ErrClosed)},
		V6: failingListener{Listener: listenOn(t, "tcp4", "127.0.0.1:0"), err: second},
	}

	assert.Equal(t, second, l.Close())
}
