package secret

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

var (
	errNoSecretFile = errors.New("no secret file found")
	errNoValue      = errors.New("no value provided for secret")
)

// valueFlags are the flags that create and update share.
type valueFlags struct {
	name     string
	value    string
	filename string
}

// flagUsage is the help text of the shared flags, which differs between create and update.
type flagUsage struct{ name, value, filename string }

// bind declares the flags on cmd and requires --name.
func (f *valueFlags) bind(cmd *cobra.Command, usage flagUsage) {
	flags := cmd.Flags()
	flags.StringVarP(&f.name, "name", "n", "", usage.name)
	flags.StringVarP(&f.value, "value", "v", "", usage.value)
	flags.StringVarP(&f.filename, "filename", "f", "", usage.filename)
	requireName(cmd)
}

// source is where the value of the secret comes from on this run: the flags and the streams of env.
func (f *valueFlags) source(env *cmdenv.Env) valueSource {
	return valueSource{
		name:        f.name,
		value:       f.value,
		filename:    f.filename,
		stdin:       env.IO.In,
		stdinTTY:    env.IO.IsStdinTTY(),
		errOut:      env.IO.ErrOut,
		warningIcon: env.IO.Style().WarningIcon(),
		readMasked:  env.IO.ReadMaskedLines,
	}
}

// read checks the name and finds the value of the secret, as create and update both begin. The errors are the ones
// the commands return.
func (f *valueFlags) read(ctx context.Context, env *cmdenv.Env) (string, error) {
	if err := validateName(f.name); err != nil {
		return "", err
	}
	value, err := resolveValue(ctx, f.source(env))
	if err != nil {
		return "", fmt.Errorf("can't read secret's value: %w", err)
	}
	return value, nil
}

// valueSource is everything resolveValue needs to find the value of one secret.
type valueSource struct {
	name        string // the secret, named in the prompt
	value       string // --value
	filename    string // --filename
	stdin       io.Reader
	stdinTTY    bool // stdin is a terminal, so the value is typed
	errOut      io.Writer
	warningIcon string
	readMasked  func(context.Context) (string, error)
}

// resolveValue returns the value of the secret: --value, else the content of --filename, else stdin. Nothing is
// trimmed or changed. Its errors are not wrapped; the command adds the context.
func resolveValue(ctx context.Context, src valueSource) (string, error) {
	switch {
	case src.value != "":
		fmt.Fprintf(src.errOut, "%s --value is visible to other users and in shell history; pipe the value on stdin or use --filename\n", src.warningIcon)
		return src.value, nil
	case src.filename != "":
		return readFile(src.filename)
	case src.stdinTTY:
		return readTyped(ctx, src)
	default:
		return readPiped(src.stdin)
	}
}

func readFile(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errNoSecretFile
		}
		return "", err
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user chooses which file holds the secret
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errNoValue
	}
	return string(data), nil
}

// readTyped asks on stderr and reads the answer with echo off. The reader leaves the Enter key's line break to
// the caller, so the next line starts clean whether or not the read worked.
func readTyped(ctx context.Context, src valueSource) (string, error) {
	fmt.Fprintf(src.errOut, "? Enter value for secret %q (end with Ctrl-D; for values over 1000 characters use --filename): ", src.name)
	value, err := src.readMasked(ctx)
	fmt.Fprintln(src.errOut)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", errNoValue
	}
	return value, nil
}

func readPiped(in io.Reader) (string, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errNoValue
	}
	return string(data), nil
}
