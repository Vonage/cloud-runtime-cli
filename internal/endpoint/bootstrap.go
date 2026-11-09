// Package endpoint turns the legacy GraphQL settings into the deployment-api bootstrap URL.
package endpoint

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Vonage/cloud-runtime-cli/internal/config"
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
	Endpoint       string // --endpoint: used as-is
	GraphQLFlag    string // --graphql-endpoint
	Region         string // --region: when set (and neither flag above is), selects ProductionDefault
	ConfigEndpoint string // endpoint from the config file: used as-is
	GraphQLConfig  string // graphql_endpoint from the config file
}

// InputsOf are the inputs the flags and the config file give. The region input is the --region flag only: a
// default_region in the file must not select the production default over the file's own endpoint.
func InputsOf(fl *config.Flags, file config.File) Inputs {
	return Inputs{
		Endpoint:       fl.Endpoint,
		GraphQLFlag:    fl.GraphQLEndpoint,
		Region:         fl.Region,
		ConfigEndpoint: file.Endpoint,
		GraphQLConfig:  file.GraphQLEndpoint,
	}
}

// Source says which input decided the URL.
type Source int

const (
	SourceDefault Source = iota
	SourceEndpointFlag
	SourceGraphQLFlag
	SourceRegionFlag
	SourceConfigEndpoint
	SourceGraphQLConfig
)

// Choice is the URL Resolve picked and where it came from. Production is true when a GraphQL setting was the
// production direct form, which needs no endpoint at all.
type Choice struct {
	URL        string
	Source     Source
	Production bool
}

// Bootstrap returns Resolve's URL.
func Bootstrap(in Inputs) (string, error) {
	c, err := Resolve(in)
	return c.URL, err
}

// Resolve returns the deployment-api base URL without the version segment:
//
//	Endpoint                                           if set
//	map(GraphQLFlag)                                   else if set
//	ProductionDefault                                  else if Region is set
//	ConfigEndpoint                                     else if set
//	map(GraphQLConfig)                                 else if set
//	ProductionDefault                                  otherwise
//
//	map("<base>/api/graphql/v1/graphql")                                  = "<base>/api/deployment"
//	map("https://graphql.<euw1|use1|apse1|apse2>.runtime.vonage.cloud/v1/graphql") = ProductionDefault
//	map(anything else)                                                   = an error (exit 1, no usage block) naming
//	                                                                       the value, the two accepted forms and
//	                                                                       --endpoint
func Resolve(in Inputs) (Choice, error) {
	switch {
	case in.Endpoint != "":
		return Choice{URL: in.Endpoint, Source: SourceEndpointFlag}, nil
	case in.GraphQLFlag != "":
		return mapped(in.GraphQLFlag, sourceFlag, SourceGraphQLFlag)
	case in.Region != "":
		// Region is the --region flag only. The configured default region never reaches here: a config that
		// points at its own endpoint would otherwise be sent to production.
		return Choice{URL: ProductionDefault, Source: SourceRegionFlag}, nil
	case in.ConfigEndpoint != "":
		return Choice{URL: in.ConfigEndpoint, Source: SourceConfigEndpoint}, nil
	case in.GraphQLConfig != "":
		return mapped(in.GraphQLConfig, sourceConfig, SourceGraphQLConfig)
	default:
		return Choice{URL: ProductionDefault, Source: SourceDefault}, nil
	}
}

func mapped(value, name string, source Source) (Choice, error) {
	url, production, err := mapGraphQL(value, name)
	if err != nil {
		return Choice{}, err
	}
	return Choice{URL: url, Source: source, Production: production}, nil
}

// mapGraphQL turns a legacy GraphQL URL into the deployment API base URL. It accepts the gateway form and the
// production direct form and nothing else. source names where the value came from, for the error.
// production reports the production direct form.
func mapGraphQL(value, source string) (base string, production bool, err error) {
	u, err := url.Parse(value)
	if err != nil {
		// The parse error quotes the whole value, credentials included, so it is not passed on.
		return "", false, unsupported(source, value)
	}
	if u.User == nil && !strings.ContainsAny(value, "?#") {
		path := strings.TrimSuffix(u.Path, "/")
		if base, ok := gatewayBase(u, path); ok {
			return base, false, nil
		}
		if isProductionDirect(u, path) {
			return ProductionDefault, true, nil
		}
	}

	shown := value
	if u.User != nil {
		shown = u.Redacted()
	}
	return "", false, unsupported(source, shown)
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
