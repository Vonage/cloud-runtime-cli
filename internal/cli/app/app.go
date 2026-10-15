// Package app is the `vcr app` group (example of the per-group shape).
package app

import (
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

// NewCmd returns `vcr app` with create, generate-keys, list and remove, annotated cmdenv.Full.
func NewCmd(env *cmdenv.Env) *cobra.Command { panic("not implemented: app.NewCmd") }
