package cli

import (
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// migrateEndpoint moves users off the GraphQL settings. A --graphql-endpoint flag gets a notice on stderr. A
// graphql_endpoint in the file gets a question on an interactive terminal and a notice everywhere else. Nothing here
// changes the command's exit code: an unmappable value is left for initPlatform to report.
func (st *runState) migrateEndpoint() {
	env := st.env
	choice, err := endpoint.Resolve(bootstrapInputs(env.Flags, env.Settings.File))
	if err != nil {
		return
	}
	warn := env.IO.Style().WarningIcon()
	switch choice.Source {
	case endpoint.SourceGraphQLFlag:
		if choice.Production {
			fmt.Fprintf(env.IO.ErrOut, "%s --graphql-endpoint is deprecated and not needed; choose the region with --region\n", warn)
			return
		}
		fmt.Fprintf(env.IO.ErrOut, "%s --graphql-endpoint is deprecated; use --endpoint %s instead\n", warn, choice.URL)
	case endpoint.SourceGraphQLConfig:
		st.migrateConfigFile(choice.URL)
	case endpoint.SourceDefault, endpoint.SourceEndpointFlag, endpoint.SourceRegionFlag, endpoint.SourceConfigEndpoint:
		// nothing to migrate
	}
}

func (st *runState) migrateConfigFile(url string) {
	env := st.env
	path := env.Settings.FilePath
	warn := env.IO.Style().WarningIcon()
	if env.IO.Mode() == term.Classic || !env.IO.CanPrompt() {
		fmt.Fprintf(env.IO.ErrOut, "%s graphql_endpoint in %s is deprecated; add \"endpoint = %s\" to the file, or run vcr configure\n",
			warn, path, url)
		return
	}
	yes, err := env.Prompt.YesNo(fmt.Sprintf("graphql_endpoint in %s is deprecated. Add \"endpoint = %s\" to the file?", path, url))
	if err != nil || !yes {
		return
	}
	if err := config.AddEndpoint(path, url); err != nil {
		fmt.Fprintf(env.IO.ErrOut, "%s Could not update %s: %v\n", warn, path, err)
		return
	}
	env.Settings.File.Endpoint = url
	fmt.Fprintf(env.IO.Out, "%s Added endpoint to %s\n", env.IO.Style().SuccessIcon(), path)
}

// bootstrapInputs are the flags and file values that choose the deployment API. The region input is the --region
// flag only: a default_region in the file must not select the production default over the file's own endpoint.
func bootstrapInputs(fl *config.Flags, file config.File) endpoint.Inputs {
	return endpoint.Inputs{
		Endpoint:       fl.Endpoint,
		GraphQLFlag:    fl.GraphQLEndpoint,
		Region:         fl.Region,
		ConfigEndpoint: file.Endpoint,
		GraphQLConfig:  file.GraphQLEndpoint,
	}
}
