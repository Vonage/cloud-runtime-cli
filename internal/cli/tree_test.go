package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deploymentAPIURLFlag = "deployment-api-url"

// flagNode describes a flag the way contract-tree.json does.
func flagNode(f *pflag.Flag, persistent bool) treeFlag {
	required := false
	if values, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; ok && len(values) > 0 {
		required = values[0] == "true"
	}
	return treeFlag{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     f.DefValue,
		Usage:       f.Usage,
		Hidden:      f.Hidden,
		Deprecated:  f.Deprecated,
		Required:    required,
		Persistent:  persistent,
		NoOptDefVal: f.NoOptDefVal,
	}
}

// localFlagNodes returns the flags a command declares itself, keyed by name.
func localFlagNodes(cmd *cobra.Command) map[string]treeFlag {
	out := map[string]treeFlag{}
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) { out[f.Name] = flagNode(f, false) })
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { out[f.Name] = flagNode(f, true) })
	return out
}

func inheritedFlagNames(cmd *cobra.Command) []string {
	var names []string
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	return names
}

func childNames(cmd *cobra.Command) []string {
	var names []string
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	return names
}

func TestContractTree(t *testing.T) {
	strict := portedAll()
	env, _, _ := newTestEnv()
	root := withBuiltins(NewRoot(env))

	var skipped []string
	isSkipped := func(path string) bool {
		return slices.ContainsFunc(skipped, func(prefix string) bool { return strings.HasPrefix(path, prefix+" ") })
	}

	for _, want := range loadContractTree(t) {
		if isSkipped(want.Path) {
			continue
		}
		cmd := findCommand(root, want.Path)
		if cmd == nil {
			if strict {
				t.Errorf("%q is missing from the command tree", want.Path)
			}
			skipped = append(skipped, want.Path)
			continue
		}
		t.Run(want.Path, func(t *testing.T) { assertMatchesNode(t, want, cmd, strict) })
	}
}

func assertMatchesNode(t *testing.T, want treeNode, got *cobra.Command, strict bool) {
	t.Helper()
	assert.Equal(t, want.Use, got.Use, "use")
	assert.ElementsMatch(t, want.Aliases, got.Aliases, "aliases")
	assert.Equal(t, want.Short, got.Short, "short")
	assert.Equal(t, want.Long, got.Long, "long")
	assert.Equal(t, want.Example, got.Example, "example")
	assert.Equal(t, want.Hidden, got.Hidden, "hidden")
	assert.Equal(t, want.Deprecated, got.Deprecated, "deprecated")
	assert.Equal(t, want.Runnable, got.Runnable(), "runnable")
	assert.Equal(t, want.ArgsValidatorSet, got.Args != nil, "argsValidatorSet")

	wantFlags := map[string]treeFlag{}
	for _, f := range want.LocalFlags {
		wantFlags[f.Name] = f
	}
	assert.Equal(t, wantFlags, localFlagNodes(got), "local flags")
	assert.ElementsMatch(t, want.InheritedFlags, inheritedFlagNames(got), "inherited flags")

	gotChildren := childNames(got)
	for _, name := range gotChildren {
		assert.Contains(t, want.Children, name, "child %q is not in the baseline", name)
	}
	if strict {
		for _, name := range want.Children {
			assert.Contains(t, gotChildren, name, "baseline child %q is missing", name)
		}
	}
}

func TestContractTreeCarriesDeploymentAPIURL(t *testing.T) {
	nodes := loadContractTree(t)

	for _, n := range nodes {
		if n.Path != rootPath {
			at := slices.Index(n.InheritedFlags, "config-file")
			require.GreaterOrEqual(t, at, 0, "%s has no inherited config-file", n.Path)
			require.Less(t, at+1, len(n.InheritedFlags), "%s lists nothing after config-file", n.Path)
			assert.Equal(t, deploymentAPIURLFlag, n.InheritedFlags[at+1], "%s", n.Path)
			continue
		}

		at := slices.IndexFunc(n.LocalFlags, func(f treeFlag) bool { return f.Name == deploymentAPIURLFlag })
		require.GreaterOrEqual(t, at, 0, "the root node has no %s flag", deploymentAPIURLFlag)
		assert.Equal(t, "config-file", n.LocalFlags[at-1].Name)
		assert.Equal(t, "graphql-endpoint", n.LocalFlags[at+1].Name)
		assert.Equal(t, treeFlag{
			Name:       deploymentAPIURLFlag,
			Type:       "string",
			Default:    "",
			Usage:      "Deployment API base URL (overrides --graphql-endpoint and the config file)",
			Hidden:     true,
			Persistent: true,
		}, n.LocalFlags[at])
	}
}
