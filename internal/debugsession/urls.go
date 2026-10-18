package debugsession

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"text/template"
)

var (
	errNoWebsocketPath = errors.New("websocket path is empty")
	errNoHost          = errors.New("host is empty")
)

// serviceAddresses are where a deployed debug service can be reached.
type serviceAddresses struct {
	Host   string   // the address of the service as its region's template wrote it: shown, and exported to the app
	Tunnel *url.URL // where the proxy accepts the tunnel
	Proxy  *url.URL // where it accepts the sockets of forwarded websockets
}

// serviceURLs fills the host template of a region with the name of the service and derives the addresses of the
// tunnel and of the proxy's sockets from it and from the websocket path the platform chose. The template is plain
// text. The URLs are built from their parts, so a ? or # in the path stays in the path.
func serviceURLs(hostTemplate, serviceName, websocketPath string) (serviceAddresses, error) {
	hostText, err := executeHostTemplate(hostTemplate, serviceName)
	if err != nil {
		return serviceAddresses{}, err
	}
	host, err := url.Parse(hostText)
	if err != nil {
		return serviceAddresses{}, err
	}
	scheme, err := websocketScheme(host.Scheme)
	if err != nil {
		return serviceAddresses{}, err
	}
	if host.Host == "" {
		return serviceAddresses{}, errNoHost
	}
	if websocketPath == "" {
		return serviceAddresses{}, errNoWebsocketPath
	}
	dir := strings.TrimSuffix(host.Path, "/")
	name := strings.TrimPrefix(websocketPath, "/")
	return serviceAddresses{
		Host:   hostText,
		Tunnel: &url.URL{Scheme: scheme, Host: host.Host, Path: dir + "/" + name},
		Proxy:  &url.URL{Scheme: scheme, Host: host.Host, Path: dir + "/_/" + name},
	}, nil
}

// websocketScheme is the websocket scheme that goes with the scheme of the host: http gives ws and https gives wss.
func websocketScheme(hostScheme string) (string, error) {
	switch hostScheme {
	case "http":
		return "ws", nil
	case "https":
		return "wss", nil
	}
	return "", fmt.Errorf("unsupported scheme %q", hostScheme)
}

func executeHostTemplate(hostTemplate, serviceName string) (string, error) {
	tmpl, err := template.New("host").Parse(hostTemplate)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, struct{ ServiceName string }{serviceName}); err != nil {
		return "", err
	}
	return out.String(), nil
}
