package cli

import (
	"context"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// migrateEndpoint moves users off the GraphQL settings. A --graphql-endpoint flag gets a notice on stderr. A
// graphql_endpoint in the file gets a question on an interactive terminal and a notice everywhere else. Nothing here
// changes the command's exit code, except an interrupt while the question waits: an unmappable value is left for
// initPlatform to report.
func (st *runState) migrateEndpoint(ctx context.Context) error {
	env := st.env
	choice, err := endpoint.Resolve(bootstrapInputs(env.Flags, env.Settings.File))
	if err != nil {
		return nil //nolint:nilerr // initPlatform reports it
	}
	warn := env.IO.Style().WarningIcon()
	switch choice.Source {
	case endpoint.SourceGraphQLFlag:
		if choice.Production {
			fmt.Fprintf(env.IO.ErrOut, "%s --graphql-endpoint is deprecated and not needed; choose the region with --region\n", warn)
			return nil
		}
		fmt.Fprintf(env.IO.ErrOut, "%s --graphql-endpoint is deprecated; use --endpoint %s instead\n", warn, choice.URL)
	case endpoint.SourceGraphQLConfig:
		return st.migrateConfigFile(ctx, choice.URL)
	case endpoint.SourceDefault, endpoint.SourceEndpointFlag, endpoint.SourceRegionFlag, endpoint.SourceConfigEndpoint:
		// nothing to migrate
	}
	return nil
}

func (st *runState) migrateConfigFile(ctx context.Context, url string) error {
	env := st.env
	path := env.Settings.FilePath
	warn := env.IO.Style().WarningIcon()
	if env.IO.Mode() == term.Classic || !env.IO.CanPrompt() {
		fmt.Fprintf(env.IO.ErrOut, "%s graphql_endpoint in %s is deprecated; add \"endpoint = %s\" to the file, or run vcr configure\n",
			warn, path, url)
		return nil
	}
	yes, err := askOrInterrupt(ctx, env.Prompt, fmt.Sprintf("graphql_endpoint in %s is deprecated. Add \"endpoint = %s\" to the file?", path, url))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !yes {
		return nil //nolint:nilerr // an unreadable answer is a no: the command still runs
	}
	if err := config.AddEndpoint(path, url); err != nil {
		fmt.Fprintf(env.IO.ErrOut, "%s Could not update %s: %v\n", warn, path, err)
		return nil
	}
	env.Settings.File.Endpoint = url
	fmt.Fprintf(env.IO.Out, "%s Added endpoint to %s\n", env.IO.Style().SuccessIcon(), path)
	return nil
}

// askOrInterrupt is YesNo that gives up when ctx ends. The read it leaves behind ends with the process.
func askOrInterrupt(ctx context.Context, p term.Prompter, question string) (bool, error) {
	type answer struct {
		yes bool
		err error
	}
	got := make(chan answer, 1)
	go func() {
		yes, err := p.YesNo(question)
		got <- answer{yes, err}
	}()
	select {
	case a := <-got:
		return a.yes, a.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
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
