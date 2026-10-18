// Package cli builds the cobra command tree and runs it. cobra is imported only under internal/cli.
package cli

import (
	"time"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/app"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/configure"
	"vonage-cloud-runtime-cli/internal/cli/debug"
	"vonage-cloud-runtime-cli/internal/cli/deploy"
	"vonage-cloud-runtime-cli/internal/cli/initcmd"
	"vonage-cloud-runtime-cli/internal/cli/instance"
	"vonage-cloud-runtime-cli/internal/cli/secret"
	"vonage-cloud-runtime-cli/internal/cli/upgrade"
	"vonage-cloud-runtime-cli/internal/config"
)

const (
	rootUse        = "vcr <command> <subcommand> [flags]"
	rootShort      = "Streamline your Vonage Cloud Runtime development and management tasks with VCR"
	defaultTimeout = 10 * time.Minute
	helpCommand    = "help"
)

// NewRoot builds the whole tree (also used by docs generation and the contract tree test). Each group package
// exposes NewCmd(env *cmdenv.Env) *cobra.Command.
func NewRoot(env *cmdenv.Env) *cobra.Command {
	opts := defaultOptions(env)
	return buildRoot(env, opts.groups, newRunState(env, opts))
}

// registeredGroups returns the command groups that have a real implementation; each command group adds its
// NewCmd(env) here. A top-level name that none of them claims gets a stand-in from pending.go.
func registeredGroups(env *cmdenv.Env) []*cobra.Command {
	return []*cobra.Command{
		app.NewCmd(env),
		configure.NewCmd(env),
		debug.NewCmd(env),
		deploy.NewCmd(env),
		initcmd.NewCmd(env),
		instance.NewCmd(env),
		secret.NewCmd(env),
		upgrade.NewCmd(env),
	}
}

// buildRoot returns the root command with groups under it, a stand-in for every top-level group that groups lacks,
// and cobra's help command. cobra adds the completion command itself when the tree runs. The root owns the only
// persistent hooks, which set up the platform before a command and show the update notice after it: a group or a
// command that defines its own would replace them, because cobra runs the nearest one only.
func buildRoot(env *cmdenv.Env, groups []*cobra.Command, st *runState) *cobra.Command {
	if env.Flags == nil {
		env.Flags = &config.Flags{}
	}
	root := &cobra.Command{
		Use:               rootUse,
		Short:             rootShort,
		Long:              rootLong,
		Example:           rootExample,
		SilenceErrors:     true,
		SilenceUsage:      true,
		PersistentPreRunE: st.preRun,
		PersistentPostRun: st.postRun,
	}
	root.SetOut(env.IO.Out)
	root.SetErr(env.IO.ErrOut)
	root.SetHelpFunc(helpFunc(env))
	root.SetUsageFunc(usageFunc)
	root.SetFlagErrorFunc(flagErrorFunc)
	bindGlobalFlags(root, env.Flags)

	root.AddCommand(groups...)
	root.AddCommand(pendingCommands(groups)...)
	root.InitDefaultHelpCmd()
	useHelpTopicRun(env, root)
	return root
}

// bindGlobalFlags declares the global flags on root and binds them to f. Only --version is local to the root.
func bindGlobalFlags(root *cobra.Command, f *config.Flags) {
	flags := root.PersistentFlags()
	flags.StringVar(&f.ConfigFile, "config-file", config.DefaultFlagValue, "Path to config file (default is $HOME/.vcr-cli)")
	flags.StringVar(&f.DeploymentAPIURL, "deployment-api-url", "",
		"Deployment API base URL (overrides --graphql-endpoint and the config file)")
	flags.StringVar(&f.GraphQLEndpoint, "graphql-endpoint", "", "Graphql endpoint used to fetch metadata")
	flags.StringVar(&f.Region, "region", "", "Vonage platform region")
	flags.StringVar(&f.APIKey, "api-key", "", "Vonage API key")
	flags.StringVar(&f.APISecret, "api-secret", "", "Vonage API secret")
	flags.DurationVarP(&f.Timeout, "timeout", "t", defaultTimeout,
		"Overall deadline for the command, including waiting for a build to finish (e.g. 20m)")
	flags.Bool("help", false, "Show help for command")
	flags.Lookup("deployment-api-url").Hidden = true

	root.Flags().BoolP("version", "v", false, "Show VCR CLI version")
}

// useHelpTopicRun swaps the run function of cobra's help command for one that prints through the env's streams.
func useHelpTopicRun(env *cmdenv.Env, root *cobra.Command) {
	for _, cmd := range root.Commands() {
		if cmd.Name() == helpCommand {
			cmd.Run = nil
			cmd.RunE = helpTopicRun(env, root)
		}
	}
}
