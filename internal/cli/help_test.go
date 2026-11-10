package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIndentAndDedent(t *testing.T) {
	t.Run("indent", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want string
		}{
			{"two lines", "a\nb", "  a\n  b"},
			{"blank line inside", "a\n\nb", "  a\n  \n  b"},
			{"final newline gets the prefix too", "a\n", "  a\n  "},
			{"empty", "", ""},
			{"only whitespace", "\n\n", "\n\n"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, indent(tt.in, "  "))
			})
		}
	})

	t.Run("dedent", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want string
		}{
			{
				"flags with and without a shorthand",
				"  -t, --timeout duration  x\n      --api-key string  y\n",
				"-t, --timeout duration  x\n    --api-key string  y\n",
			},
			{"no common indent", "a\n b", "a\n b"},
			{"only a long flag", "      --a  x\n", "--a  x\n"},
			{"blank lines stay blank", "  a\n\n  b", "a\n\nb"},
			{"empty", "", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, dedent(tt.in))
			})
		}
	})
}

func TestCommandListPadding(t *testing.T) {
	assert.Equal(t, "create:      ", rpad("create:"))
	assert.Equal(t, "generate-keys: ", rpad("generate-keys:"))
}

func TestHelpFallsBackToShortText(t *testing.T) {
	const wantStart = "Do the thing\n\nUSAGE\n  vcr thing [flags]\n\nINHERITED FLAGS\n"
	env, stdout, stderr := newTestEnv()
	thing := &cobra.Command{Use: "thing", Short: "Do the thing", Run: func(*cobra.Command, []string) {}}
	root := buildTestRoot(env, []*cobra.Command{thing})

	code := executeTestRoot(t, root, env, []string{"thing", "--help"})

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stdout.String(), wantStart), "stdout = %q", stdout.String())
}

func TestUsageText(t *testing.T) {
	noop := func(*cobra.Command, []string) {}

	tests := []struct {
		name  string
		build func() *cobra.Command
		want  string
	}{
		{
			name: "a command with subcommands lists their names",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "app", Run: noop}
				cmd.AddCommand(
					&cobra.Command{Use: "create", Run: noop},
					&cobra.Command{Use: "list", Run: noop},
					&cobra.Command{Use: "hidden", Hidden: true, Run: noop},
				)
				return cmd
			},
			want: "Usage:  vcr app [flags]\n\nAvailable commands:\n  create\n  list\n",
		},
		{
			name: "a leaf lists its own flags",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "thing", Run: noop}
				cmd.Flags().StringP("foo", "f", "", "Foo usage")
				return cmd
			},
			want: "Usage:  vcr thing [flags]\n\nFlags:\n  -f, --foo string   Foo usage\n  ",
		},
		{
			name: "a leaf without local flags is the use line alone",
			build: func() *cobra.Command {
				return &cobra.Command{Use: "thing", Run: noop}
			},
			want: "Usage:  vcr thing [flags]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _, _ := newTestEnv()
			cmd := tt.build()
			buildTestRoot(env, []*cobra.Command{cmd})

			assert.Equal(t, tt.want, usageText(cmd))
		})
	}
}
