// Package endpoint turns the legacy GraphQL settings into the deployment-api bootstrap URL.
package endpoint

// ProductionDefault is the bootstrap used when nothing else is configured.
const ProductionDefault = "https://api-eu.vonage.com/v1/vcr/euw1/api/deployment"

// Inputs are the values that choose the environment, in the order they win.
type Inputs struct {
	DeploymentAPIURL string // hidden --deployment-api-url: used as-is
	GraphQLFlag      string // --graphql-endpoint
	Region           string // --region: when set (and GraphQLFlag is not), selects ProductionDefault
	GraphQLConfig    string // graphql_endpoint from the config file
}

// Bootstrap returns the deployment-api base URL without the version segment:
//
//	DeploymentAPIURL                                   if set
//	map(GraphQLFlag)                                   else if set
//	ProductionDefault                                  else if Region is set
//	map(GraphQLConfig)                                 else if set
//	ProductionDefault                                  otherwise
//
//	map("<base>/api/graphql/v1/graphql")                                  = "<base>/api/deployment"
//	map("https://graphql.<euw1|use1|apse1|apse2>.runtime.vonage.cloud/v1/graphql") = ProductionDefault
//	map(anything else)                                                   = an error (exit 1, no usage block) naming
//	                                                                       the value, the two accepted forms and
//	                                                                       --deployment-api-url
func Bootstrap(in Inputs) (string, error) { panic("not implemented: endpoint.Bootstrap") }

// ValidateRegion returns `invalid --region "<value>": expected aws.<name>` unless alias matches ^aws\.[a-z0-9]+$
// An empty alias is valid here (it means "not given").
func ValidateRegion(alias string) error { panic("not implemented: endpoint.ValidateRegion") }
