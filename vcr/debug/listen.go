package debug

import (
	"errors"
	"net"
	"strconv"
	"syscall"
)

const (
	// The Windows socket errors for "address family not supported" and "address not available".
	errWSAEAFNoSupport  = syscall.Errno(10047)
	errWSAEAddrNotAvail = syscall.Errno(10049)
)

// listenFunc binds a socket; tests pass their own to simulate bind errors.
type listenFunc func(network, address string) (net.Listener, error)

// listenLoopback binds the local debugger's sockets on 127.0.0.1:<port> and [::1]:<port>. Any IPv4 error is fatal. On
// IPv6 only "not available" is tolerated, and the result then holds the IPv4 listener alone.
func listenLoopback(listen listenFunc, port int) ([]net.Listener, error) {
	v4, err := listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	v6, err := listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil {
		if ipv6Unavailable(err) {
			return []net.Listener{v4}, nil
		}
		// A process that holds the IPv6 port would otherwise get the SDK's traffic unnoticed.
		_ = v4.Close()
		return nil, err
	}
	return []net.Listener{v4, v6}, nil
}

// ipv6Unavailable reports whether err only says that this machine has no IPv6 loopback address: the family is not
// supported or the address cannot be assigned, on Unix and with the Windows socket error codes.
func ipv6Unavailable(err error) bool {
	return errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) ||
		errors.Is(err, errWSAEAFNoSupport) || errors.Is(err, errWSAEAddrNotAvail)
}
