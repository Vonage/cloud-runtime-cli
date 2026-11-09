package debug

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/debugsession"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	instanceAppQuestion = "Are you sure you want to debug with instance app id ?"
	pickerMessage       = "Select your Vonage application ID for debug session:"

	// skipLabel keeps the application, and newAppLabel stands for none: the platform rejects the empty id.
	skipLabel   = "SKIP"
	newAppLabel = "CREATE NEW APP"
)

var (
	errApplicationRequired = errors.New("application-id is required")
	errRuntimeRequired     = errors.New("runtime is required")
)

// invocation is one run of `vcr debug`: the flags it got, the environment it runs in and the function that runs the
// session.
type invocation struct {
	env   *cmdenv.Env
	flags *flags
	run   runFunc
}

// execute settles everything the session needs from the flags, the manifest and the config, and then runs it. Until
// the session starts, the steps run under the command's context, which carries the --timeout deadline; the session
// gets the signal context instead, because it ends only with Ctrl-C.
func (in *invocation) execute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dir, m, err := in.loadProject(args)
	if err != nil {
		return err
	}
	region := chooseRegion(in.env.Flags.Region, m, in.env.Settings.Region)
	client, err := in.env.RegionClient(ctx, region.alias)
	if err != nil {
		return region.wrap(err)
	}
	cfg, err := in.config(ctx, client, dir, m)
	if err != nil {
		return err
	}
	return in.run(cmdenv.SignalContext(ctx), in.env.IO, in.env.Clock, client, cfg)
}

// loadProject reads the manifest of the project directory.
func (in *invocation) loadProject(args []string) (dir string, m *manifest.Manifest, err error) {
	var arg string
	if len(args) > 0 {
		arg = args[0]
	}
	dir, err = manifest.ProjectDir(arg)
	if err != nil {
		return "", nil, err
	}
	path, err := manifest.Find(dir, in.flags.filename)
	if err != nil {
		return "", nil, err
	}
	m, err = manifest.Read(path)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read manifest file: %w", err)
	}
	return dir, m, nil
}

// config is what the session runs with. The application and the runtime are required; the name is not, because the
// platform picks one.
func (in *invocation) config(ctx context.Context, client *platform.Client, dir string, m *manifest.Manifest) (debugsession.Config, error) {
	appID, err := in.applicationID(m)
	if err != nil {
		return debugsession.Config{}, fmt.Errorf("failed to get debug app id: %w", err)
	}
	runtime := firstOf(in.flags.runtime, m.Instance.Runtime)
	if runtime == "" {
		return debugsession.Config{}, fmt.Errorf("failed to get runtime: %w", errRuntimeRequired)
	}
	if appID, err = in.confirmApplication(ctx, client, appID, m.Instance.ApplicationID); err != nil {
		return debugsession.Config{}, err
	}
	env := in.env
	return debugsession.Config{
		ProjectDir:   dir,
		Manifest:     m,
		AppID:        appID,
		Runtime:      runtime,
		Name:         firstOf(in.flags.name, m.Debug.Name),
		AppPort:      in.flags.appPort,
		DebuggerPort: in.flags.debuggerPort,
		PreserveData: in.flags.preserveData || m.Debug.PreserveData, // the flag can never switch it off
		Verbose:      in.flags.verbose,
		Creds:        platform.Credentials{APIKey: env.Settings.APIKey, APISecret: env.Settings.APISecret},
		Timeout:      env.Flags.Timeout,
	}, nil
}

// applicationID is the flag, else the debug application of the manifest, else the application of the instance, which
// is announced because a debug session usually has one of its own.
func (in *invocation) applicationID(m *manifest.Manifest) (string, error) {
	if id := firstOf(in.flags.appID, m.Debug.ApplicationID); id != "" {
		return id, nil
	}
	if m.Instance.ApplicationID == "" {
		return "", errApplicationRequired
	}
	fmt.Fprintf(in.env.IO.Out, "%s A debug application-id was not detected in the manifest, "+
		"the instance application-id was loaded as an alternative. Please consider adding a debug application-id\n",
		in.env.IO.Style().WarningIcon())
	return m.Instance.ApplicationID, nil
}

// confirmApplication asks, on a terminal, whether to debug with the application of the instance, and offers the other
// applications of the account when the answer is no.
func (in *invocation) confirmApplication(ctx context.Context, client *platform.Client, appID, instanceAppID string) (string, error) {
	if !in.env.IO.CanPrompt() || appID != instanceAppID || in.flags.yes {
		return appID, nil
	}
	sure, err := in.env.Prompt.YesNo(instanceAppQuestion)
	if err != nil {
		return "", err
	}
	if sure {
		return appID, nil
	}
	return in.pickApplication(ctx, client, appID)
}

// pickApplication lets the user choose one of the applications of the account. SKIP keeps current, and CREATE NEW APP
// has no id: the platform rejects the empty one, as it always did.
func (in *invocation) pickApplication(ctx context.Context, client *platform.Client, current string) (string, error) {
	apps, err := in.fetchApplications(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to list Vonage applications: %w", err)
	}
	labels, ids := applicationChoices(apps)
	label, err := in.env.Prompt.Select(pickerMessage, labels, "")
	if err != nil {
		return "", fmt.Errorf("failed to select Vonage application: %w", err)
	}
	if label == skipLabel {
		return current, nil
	}
	return ids[label], nil
}

// fetchApplications lists the applications of the account behind a spinner.
func (in *invocation) fetchApplications(ctx context.Context, client *platform.Client) ([]platform.Application, error) {
	if in.env.IO.Mode() != term.Classic {
		return in.fetchApplicationsRich(ctx, client)
	}
	stop := in.env.IO.StartSpinner(" Fetching applications list...")
	apps, err := client.ListApplications(ctx, "")
	stop()
	return apps, err
}

// applicationChoices are the labels of the picker, in the order of the platform's list after SKIP and CREATE NEW APP,
// and the id behind each application's label. The names and ids are the platform's text, so the labels are sanitised.
func applicationChoices(apps []platform.Application) (labels []string, ids map[string]string) {
	labels = append(make([]string, 0, len(apps)+2), skipLabel, newAppLabel)
	ids = make(map[string]string, len(apps))
	for _, app := range apps {
		label := term.Sanitize(fmt.Sprintf("%s - (%s)", app.Name, app.ID))
		labels = append(labels, label)
		ids[label] = app.ID
	}
	return labels, ids
}

// firstOf is the first value that is not empty.
func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
