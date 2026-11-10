package debugsession

import (
	"net/http"
	"strconv"
	"strings"
)

// LocalOnly wraps the debugger handler: 403 unless Host is localhost, 127.0.0.1 or [::1] with the port (bare form
// when the port is 80), and 403 when the request has an Origin or a Sec-Fetch-Site header.
func LocalOnly(port int, next http.Handler) http.Handler {
	allowed := localHosts(port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.Host is the host Go parsed: the Host header, or the target's own host for an absolute-form request.
		if !allowed[strings.ToLower(r.Host)] || fromBrowser(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// localHosts are the Host values that name this machine's debugger. A request that carries any other name reached the
// port through something else, such as a DNS name an attacker's page rebound to 127.0.0.1.
func localHosts(port int) map[string]bool {
	hosts := make(map[string]bool, 6)
	for _, name := range []string{"localhost", "127.0.0.1", "[::1]"} {
		hosts[name+":"+strconv.Itoa(port)] = true
		if port == 80 {
			hosts[name] = true
		}
	}
	return hosts
}

// fromBrowser reports whether the request carries a header that browsers add to the requests of web pages and that
// the SDKs never send. Sec-Fetch-Mode and Sec-Fetch-Dest say nothing here: Node's built-in fetch sends both.
func fromBrowser(r *http.Request) bool {
	return len(r.Header.Values("Origin")) > 0 || len(r.Header.Values("Sec-Fetch-Site")) > 0
}
