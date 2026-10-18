package cli

import (
	"errors"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

// pendingAnnotation marks a stand-in for a command group that is not implemented yet.
const pendingAnnotation = "pending"

// pendingGroup is the baseline Use, Aliases and Short of a top-level group.
type pendingGroup struct {
	use     string
	aliases []string
	short   string
}

var pendingGroups = []pendingGroup{
	{use: "app <command>", short: "Manage Vonage applications for VCR deployments"},
	{use: "configure", short: "Configure VCR CLI with your Vonage API credentials"},
	{use: "debug [path_to_project]", short: "Run your application locally in debug mode with live VCR integration"},
	{use: "deploy [path_to_code]", short: "Deploy your application to Vonage Cloud Runtime"},
	{use: "init [path_to_project]", aliases: []string{"i"}, short: "Initialize a new VCR project from a template"},
	{use: "instance <command>", short: "Manage deployed VCR instances"},
	{use: "secret <command>", short: "Manage secrets for VCR applications"},
	{use: "upgrade", short: "Check for and install VCR CLI updates"},
}

var errNotAvailableYet = errors.New("this command is not available in this build yet")

// pendingCommands returns a stand-in for every top-level group that none of claimed provides, so the root page and
// the usage block already list all of them.
func pendingCommands(claimed []*cobra.Command) []*cobra.Command {
	var stubs []*cobra.Command
	for _, group := range pendingGroups {
		name, _, _ := strings.Cut(group.use, " ")
		if slices.ContainsFunc(claimed, func(c *cobra.Command) bool { return c.Name() == name }) {
			continue
		}
		stubs = append(stubs, &cobra.Command{
			Use:     group.use,
			Aliases: group.aliases,
			Short:   group.short,
			Annotations: map[string]string{
				cmdenv.Annotation: cmdenv.None,
				pendingAnnotation: "true",
			},
			RunE: func(*cobra.Command, []string) error { return errNotAvailableYet },
		})
	}
	return stubs
}
