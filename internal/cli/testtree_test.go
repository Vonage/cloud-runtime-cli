package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	contractTreeFile = "../../test/contract/testdata/contract-tree.json"
	goldenHelpDir    = "../../test/contract/testdata/golden-help"
	rootPath         = "vcr"
	helpPath         = "vcr help"
	completionPath   = "vcr completion"
)

// treeFlag and treeNode are the entries of contract-tree.json. The file leaves out every empty field, so a missing
// key reads as the zero value.
type treeFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Usage       string `json:"usage"`
	Hidden      bool   `json:"hidden"`
	Deprecated  string `json:"deprecated"`
	Required    bool   `json:"required"`
	Persistent  bool   `json:"persistent"`
	NoOptDefVal string `json:"noOptDefVal"`
}

type treeNode struct {
	Path             string     `json:"path"`
	Use              string     `json:"use"`
	Aliases          []string   `json:"aliases"`
	Short            string     `json:"short"`
	Long             string     `json:"long"`
	Example          string     `json:"example"`
	Hidden           bool       `json:"hidden"`
	Deprecated       string     `json:"deprecated"`
	ArgsValidatorSet bool       `json:"argsValidatorSet"`
	Runnable         bool       `json:"runnable"`
	LocalFlags       []treeFlag `json:"localFlags"`
	InheritedFlags   []string   `json:"inheritedFlags"`
	Children         []string   `json:"children"`
}

func loadContractTree(t *testing.T) []treeNode {
	t.Helper()
	raw, err := os.ReadFile(contractTreeFile)
	require.NoError(t, err)
	var nodes []treeNode
	require.NoError(t, json.Unmarshal(raw, &nodes))
	require.NotEmpty(t, nodes)
	return nodes
}

func nodesByPath(nodes []treeNode) map[string]treeNode {
	out := make(map[string]treeNode, len(nodes))
	for _, n := range nodes {
		out[n.Path] = n
	}
	return out
}

// newTestEnv returns an env over buffers and those buffers.
func newTestEnv() (env *cmdenv.Env, stdout, stderr *bytes.Buffer) {
	streams, out, errOut := term.Test("", false)
	env = &cmdenv.Env{
		IO:     streams,
		Prompt: streams.Prompter(),
		Clock:  clock.System(),
		Build:  testBuildInfo("v2.6.1"),
		Flags:  &config.Flags{},
	}
	return env, out, errOut
}

// buildTestRoot is the only place the tests call buildRoot.
func buildTestRoot(env *cmdenv.Env, groups []*cobra.Command) *cobra.Command {
	return buildRoot(env, groups, newRunState(env, defaultOptions(env)))
}

// executeTestRoot runs args on root the way Run does and returns the exit code.
func executeTestRoot(t *testing.T, root *cobra.Command, env *cmdenv.Env, args []string) int {
	t.Helper()
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(t.Context())
	if err != nil {
		clierr.Render(env.IO, err, usageText(cmd))
		return 1
	}
	return 0
}

// withBuiltins adds the two commands cobra builds on its own: help and completion.
func withBuiltins(root *cobra.Command) *cobra.Command {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	return root
}

// findCommand walks the command names of path (the first word is the root) and returns nil when one is missing.
func findCommand(root *cobra.Command, path string) *cobra.Command {
	words := strings.Fields(path)
	if len(words) == 0 || words[0] != root.Name() {
		return nil
	}
	cmd := root
	for _, word := range words[1:] {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == word {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cmd = next
	}
	return cmd
}

func isBuiltinPath(path string) bool {
	return path == rootPath || path == helpPath || path == completionPath || strings.HasPrefix(path, completionPath+" ")
}

// syntheticGroups builds a cobra tree from the baseline nodes for every top-level command except help and
// completion (cobra builds those itself). The commands carry the baseline texts and flags and do nothing.
func syntheticGroups(t *testing.T, nodes []treeNode) []*cobra.Command {
	t.Helper()
	byPath := map[string]*cobra.Command{}
	var groups []*cobra.Command
	for _, n := range nodes {
		if isBuiltinPath(n.Path) {
			continue
		}
		cmd := &cobra.Command{
			Use:        n.Use,
			Aliases:    n.Aliases,
			Short:      n.Short,
			Long:       n.Long,
			Example:    n.Example,
			Hidden:     n.Hidden,
			Deprecated: n.Deprecated,
		}
		if n.Runnable {
			cmd.Run = func(*cobra.Command, []string) {}
		}
		addTestFlags(t, cmd, n.LocalFlags)
		byPath[n.Path] = cmd

		parent := n.Path[:strings.LastIndex(n.Path, " ")]
		if parent == rootPath {
			groups = append(groups, cmd)
			continue
		}
		owner, ok := byPath[parent]
		require.True(t, ok, "the parent of %q comes after it in the tree file", n.Path)
		owner.AddCommand(cmd)
	}
	return groups
}

func addTestFlags(t *testing.T, cmd *cobra.Command, flags []treeFlag) {
	t.Helper()
	for _, f := range flags {
		switch f.Type {
		case "bool":
			cmd.Flags().BoolP(f.Name, f.Shorthand, f.Default == "true", f.Usage)
		case "string":
			cmd.Flags().StringP(f.Name, f.Shorthand, f.Default, f.Usage)
		case "int":
			value, err := strconv.Atoi(f.Default)
			require.NoError(t, err, "default of flag %s", f.Name)
			cmd.Flags().IntP(f.Name, f.Shorthand, value, f.Usage)
		default:
			t.Fatalf("flag %s has the kind %q, which the test tree does not build", f.Name, f.Type)
		}
		if f.Required {
			require.NoError(t, cmd.MarkFlagRequired(f.Name))
		}
	}
}

const (
	rootUsageBlock = "Usage:  vcr <command> <subcommand> [flags]\n\nAvailable commands:\n" +
		"  app\n  completion\n  configure\n  debug\n  deploy\n  init\n  instance\n  secret\n  upgrade\n"
	completionUsageBlock = "Usage:  vcr completion [flags]\n\nAvailable commands:\n" +
		"  bash\n  fish\n  powershell\n  zsh\n"
	completionBashUsageBlock = "Usage:  vcr completion bash\n\nFlags:\n  --no-descriptions   disable completion descriptions\n  "
)

func testBuildInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:    version,
		Commit:     "0123456789abcdef",
		Date:       "2030-01-02T03:04:05Z",
		APIVersion: "v0.3",
		ReleaseURL: "https://api.example.test/repos/o/r",
	}
}

// runCLI runs args through Run and returns the exit code and what each stream received.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	streams, out, errOut := term.Test("", false)
	code = Run(t.Context(), args, streams, testBuildInfo("v2.7.0"))
	return code, out.String(), errOut.String()
}
