// Package endpoint turns the legacy GraphQL settings into the deployment-api bootstrap URL.
package endpoint

import (
	"fmt"
	"net/url"
	"strings"
)

// ProductionDefault is the bootstrap used when nothing else is configured.
const ProductionDefault = "https://api-eu.vonage.com/v1/vcr/euw1/api/deployment"

const (
	sourceFlag   = "--graphql-endpoint"
	sourceConfig = "graphql_endpoint"

	schemeHTTP  = "http"
	schemeHTTPS = "https"

	gatewaySuffix    = "/api/graphql/v1/graphql"
	deploymentSuffix = "/api/deployment"

	productionHostPrefix = "graphql."
	productionHostSuffix = ".runtime.vonage.cloud"
	productionPath       = "/v1/graphql"
)

// productionLabels are the regions the production direct form may name.
var productionLabels = []string{"euw1", "use1", "apse1", "apse2"}

// Inputs are the values that choose the environment, in the order they win.
type Inputs struct {
	Endpoint      string // hidden --endpoint: used as-is
	GraphQLFlag   string // --graphql-endpoint
	Region        string // --region: when set (and GraphQLFlag is not), selects ProductionDefault
	GraphQLConfig string // graphql_endpoint from the config file
}

// Bootstrap returns the deployment-api base URL without the version segment:
//
//	Endpoint                                           if set
//	map(GraphQLFlag)                                   else if set
//	ProductionDefault                                  else if Region is set
//	map(GraphQLConfig)                                 else if set
//	ProductionDefault                                  otherwise
//
//	map("<base>/api/graphql/v1/graphql")                                  = "<base>/api/deployment"
//	map("https://graphql.<euw1|use1|apse1|apse2>.runtime.vonage.cloud/v1/graphql") = ProductionDefault
//	map(anything else)                                                   = an error (exit 1, no usage block) naming
//	                                                                       the value, the two accepted forms and
//	                                                                       --endpoint
func Bootstrap(in Inputs) (string, error) {
	switch {
	case in.Endpoint != "":
		return in.Endpoint, nil
	case in.GraphQLFlag != "":
		return mapGraphQL(in.GraphQLFlag, sourceFlag)
	case in.Region != "":
		// Region is the --region flag only. The configured default region never reaches here: a config that
		// points at its own endpoint would otherwise be sent to production.
		return ProductionDefault, nil
	case in.GraphQLConfig != "":
		return mapGraphQL(in.GraphQLConfig, sourceConfig)
	default:
		return ProductionDefault, nil
	}
}

// mapGraphQL turns a legacy GraphQL URL into the deployment API base URL. It accepts the gateway form and the
// production direct form and nothing else. source names where the value came from, for the error.
func mapGraphQL(value, source string) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		// The parse error quotes the whole value, credentials included, so it is not passed on.
		return "", unsupported(source, value)
	}
	if u.User == nil && !strings.ContainsAny(value, "?#") {
		path := strings.TrimSuffix(u.Path, "/")
		if base, ok := gatewayBase(u, path); ok {
			return base, nil
		}
		if isProductionDirect(u, path) {
			return ProductionDefault, nil
		}
	}

	shown := value
	if u.User != nil {
		shown = u.Redacted()
	}
	return "", unsupported(source, shown)
}

// gatewayBase maps "<scheme>://<host><prefix>/api/graphql/v1/graphql" to "<scheme>://<host><prefix>/api/deployment".
func gatewayBase(u *url.URL, path string) (string, bool) {
	if (u.Scheme != schemeHTTPS && u.Scheme != schemeHTTP) || u.Hostname() == "" {
		return "", false
	}
	prefix, ok := strings.CutSuffix(path, gatewaySuffix)
	if !ok {
		return "", false
	}
	base := url.URL{Scheme: u.Scheme, Host: u.Host, Path: prefix + deploymentSuffix}
	return base.String(), true
}

// isProductionDirect reports whether u is https://graphql.<label>.runtime.vonage.cloud/v1/graphql for one of the
// production labels. u.Host carries any port, so a port never matches.
func isProductionDirect(u *url.URL, path string) bool {
	if u.Scheme != schemeHTTPS || path != productionPath {
		return false
	}
	for _, label := range productionLabels {
		want := productionHostPrefix + label + productionHostSuffix
		// EqualFold also folds non-ASCII letters (U+017F is an s). Those take more bytes than the letter they
		// imitate, so the length check keeps the match exact.
		if len(u.Host) == len(want) && strings.EqualFold(u.Host, want) {
			return true
		}
	}
	return false
}

func unsupported(source, value string) error {
	return fmt.Errorf("unsupported %s %q: expected a URL ending in /api/graphql/v1/graphql, "+
		"or https://graphql.<%s>.runtime.vonage.cloud/v1/graphql; alternatively pass --endpoint",
		source, value, strings.Join(productionLabels, "|"))
}
