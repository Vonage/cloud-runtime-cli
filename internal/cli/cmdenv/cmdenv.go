// Package cmdenv is what every command constructor receives. The root's PersistentPreRunE fills the platform fields
// once, before any RunE runs; commands only read them.
package cmdenv

import (
	"context"
	"errors"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

// ErrRegionNotFound is the error RegionClient returns for an empty or unknown alias. Its text is v2.6.1's.
var ErrRegionNotFound = errors.New("region does not exist")

// Annotation is the cobra annotation key each command sets to one of the values below.
const Annotation = "platform"

const (
	// Full: PersistentPreRunE loads config (printing today's warnings), shows the ` Executing cmd "<name>"...` spinner
	// on a TTY, resolves the region record for Settings.Region and starts the update check (app, secret, instance, init).
	Full = "full"
	// ConfigOnly: as Full without the region lookup; the command resolves its region itself, after reading the
	// manifest (deploy, debug).
	ConfigOnly = "config-only"
	// None: no config, no network in PersistentPreRunE (help, completion, --version, configure, upgrade).
	None = "none"
)

// Env is shared by all commands of one run.
type Env struct {
	IO     *term.IO
	Prompt term.Prompter
	Clock  clock.Clock
	Build  buildinfo.Info
	Flags  *config.Flags // bound to the root's persistent flags

	// Set by PersistentPreRunE for Full and ConfigOnly commands.
	Settings config.Settings
	Resolver *platform.Resolver
	// Set by PersistentPreRunE for Full commands only: the client of Settings.Region.
	Client *platform.Client
}

// WithSignalContext returns ctx carrying signal, the context that only Ctrl-C or SIGTERM cancels (no --timeout
// deadline). cli.Run calls it before it applies the --timeout deadline.
func WithSignalContext(ctx, signal context.Context) context.Context {
	panic("not implemented: cmdenv.WithSignalContext")
}

// SignalContext returns the signal-only context stored by WithSignalContext, or ctx itself when there is none.
// The phases that v2.6.1 never bounded by --timeout use it: the `instance log --follow` loop (each poll still gets its
// own --timeout) and the `vcr debug` session after setup. Everything else runs under cmd.Context()'s deadline.
func SignalContext(ctx context.Context) context.Context {
	panic("not implemented: cmdenv.SignalContext")
}

// RegionClient resolves alias the way v2.6.1's start-up did. An empty alias first calls Resolver.Regions, so bad
// credentials still fail with the platform's error, and then fails with ErrRegionNotFound; an unknown alias fails
// with ErrRegionNotFound; other errors are returned unchanged. Callers wrap the error as
// "failed to initialize cli: %w". Used by the root's pre-run for full commands, and by deploy and debug.
func (e *Env) RegionClient(ctx context.Context, alias string) (*platform.Client, error) {
	panic("not implemented: cmdenv.Env.RegionClient")
}
