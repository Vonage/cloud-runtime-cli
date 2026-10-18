// Package deploy is the `vcr deploy` command.
package deploy

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	stages "vonage-cloud-runtime-cli/internal/deploy"
	"vonage-cloud-runtime-cli/internal/manifest"
)

// flagValues are what the seven local flags hold.
type flagValues struct {
	projectName  string
	runtime      string
	appID        string
	instanceName string
	capabilities string
	tgz          string
	filename     string
}

// NewCmd returns `vcr deploy`. It resolves its region itself, after it has read the manifest, so it asks for no
// region from the root.
func NewCmd(env *cmdenv.Env) *cobra.Command {
	var f flagValues
	cmd := &cobra.Command{
		Use:         "deploy [path_to_code]",
		Short:       "Deploy your application to Vonage Cloud Runtime",
		Long:        longText,
		Args:        cobra.MaximumNArgs(1),
		Example:     exampleText,
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.ConfigOnly},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, env, f, args)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&f.projectName, "project-name", "p", "", "Project name (overrides manifest value)")
	flags.StringVarP(&f.runtime, "runtime", "r", "", "Runtime environment, e.g., nodejs18, nodejs20, python3 (overrides manifest)")
	flags.StringVarP(&f.appID, "app-id", "i", "", "Vonage application UUID to link with this deployment (overrides manifest)")
	flags.StringVarP(&f.instanceName, "instance-name", "n", "", "Instance name, e.g., dev, staging, prod (overrides manifest)")
	flags.StringVarP(&f.capabilities, "capabilities", "c", "", "Comma-separated capabilities: messages-v1,voice,rtc (overrides manifest)")
	flags.StringVarP(&f.tgz, "tgz", "z", "", "Path to pre-compressed tar.gz file to deploy (skips local compression)")
	flags.StringVarP(&f.filename, "filename", "f", "", "Path to manifest file (default: vcr.yml in project directory)")
	return cmd
}

// run reads the manifest, finds the region to deploy to and hands the plan to the stages. Nothing is requested
// before the manifest has been read.
func run(cmd *cobra.Command, env *cmdenv.Env, f flagValues, args []string) error {
	var arg string
	if len(args) > 0 {
		arg = args[0]
	}
	dir, err := manifest.ProjectDir(arg)
	if err != nil {
		return err
	}
	path, err := manifest.Find(dir, f.filename)
	if err != nil {
		return err
	}
	read, err := manifest.Read(path)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	region, source := effectiveRegion(env.Flags.Region, read, env.Settings.File.DefaultRegion)
	client, err := env.RegionClient(cmd.Context(), region)
	if err != nil {
		// v2.6.1 looked up the manifest's region only after start-up, when it built the deployment client, so an
		// unknown one from the manifest fails under that name; every other failure is a failure to start up.
		prefix := "failed to initialize cli"
		if source == fromManifest && errors.Is(err, cmdenv.ErrRegionNotFound) {
			prefix = "failed to initialize deployment client"
		}
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return stages.Run(cmd.Context(), env.IO, client, buildPlan(f, read, dir, region))
}
