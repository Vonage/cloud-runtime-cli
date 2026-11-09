package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// preRun is the root's PersistentPreRunE: it sets up what the command runs on, as far as its platform mode asks.
// cobra runs it before it checks required flags, so a start-up failure shows before a missing-flag error and
// before any prompt.
func (st *runState) preRun(cmd *cobra.Command, _ []string) error {
	env := st.env
	env.Flags.ConfigFileSet = cmd.Flags().Changed("config-file")
	if err := endpoint.ValidateRegion(env.Flags.Region); err != nil {
		return err
	}
	mode, err := modeOf(cmd)
	if err != nil {
		return err
	}
	if mode == cmdenv.None {
		st.applyDeadline(cmd)
		return nil
	}
	if err := st.loadConfig(); err != nil {
		return err
	}
	// The migration question comes before the deadline starts, so the time a person takes to answer is not taken
	// from --timeout; an interrupt still ends it.
	if err := st.migrateEndpoint(cmd.Context()); err != nil {
		return err
	}
	ctx := st.applyDeadline(cmd)
	stop := startConnecting(env, cmd, connectingRegion(env, mode))
	defer stop()
	if err := st.initPlatform(ctx, mode); err != nil {
		return fmt.Errorf("failed to initialize cli: %w", err)
	}
	st.check = startUpdateCheck(ctx, st.opts.updates(env.Build), env.Build.Version)
	return nil
}

// applyDeadline makes --timeout the deadline of cmd's context. A zero or negative timeout has expired already.
func (st *runState) applyDeadline(cmd *cobra.Command) context.Context {
	ctx, cancel := context.WithTimeout(cmd.Context(), st.env.Flags.Timeout)
	st.cancel = cancel
	cmd.SetContext(ctx)
	return ctx
}

// modeOf is the platform mode of cmd: the annotation of the nearest command that has one, or none (cobra's own help
// and completion commands carry no annotation).
func modeOf(cmd *cobra.Command) (string, error) {
	for c := cmd; c != nil; c = c.Parent() {
		mode, ok := c.Annotations[cmdenv.Annotation]
		if !ok {
			continue
		}
		switch mode {
		case cmdenv.Full, cmdenv.ConfigOnly, cmdenv.None:
			return mode, nil
		}
		return "", fmt.Errorf("command %q has the unknown platform mode %q", c.CommandPath(), mode)
	}
	return cmdenv.None, nil
}

// loadConfig reads the config file and prints its warnings on stdout, as the first thing a command shows.
func (st *runState) loadConfig() error {
	env := st.env
	settings, warnings, err := config.Load(*env.Flags)
	if err != nil {
		return err
	}
	icon := env.IO.Style().WarningIcon()
	for _, warning := range warnings {
		fmt.Fprintf(env.IO.Out, "%s %s\n", icon, warning)
	}
	env.Settings = settings
	return nil
}

// initPlatform picks the deployment API to ask, builds the resolver and, for a full command, resolves the region.
func (st *runState) initPlatform(ctx context.Context, mode string) error {
	env := st.env
	bootstrap, err := endpoint.Bootstrap(bootstrapInputs(env.Flags, env.Settings.File))
	if err != nil {
		return err
	}
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  bootstrap,
		APIVersion: env.Build.APIVersion,
		Creds:      platform.Credentials{APIKey: env.Settings.APIKey, APISecret: env.Settings.APISecret},
		UserAgent:  env.Build.UserAgent(),
		Clock:      env.Clock,
		Notices:    env.IO.ErrOut,
	})
	if err != nil {
		return err
	}
	env.Resolver = resolver
	if mode == cmdenv.Full {
		return st.resolveRegion(ctx)
	}
	return nil
}

// resolveRegion looks up the region of the settings, with the credentials the resolver carries, and keeps its client.
func (st *runState) resolveRegion(ctx context.Context) error {
	client, err := st.env.RegionClient(ctx, st.env.Settings.Region)
	if err != nil {
		return err
	}
	st.env.Client = client
	return nil
}

func spinnerMessage(cmd *cobra.Command) string {
	return fmt.Sprintf(" Executing cmd %q...", cmd.Name())
}
