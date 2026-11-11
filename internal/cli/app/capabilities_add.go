package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const capabilitiesKey = "application-capabilities"

var (
	errNoCapability = errors.New("no capability given: pass one or more of " +
		"--voice, --messages, --rtc, --verify, --video, --vbc, --meetings, --network-apis")
	errNetworkAPIsValues  = errors.New("--network-apis needs --network-application-id and --redirect-uri")
	errNetworkAPIsMissing = errors.New("--network-application-id and --redirect-uri need --network-apis")
)

// capabilitiesAddFlags are the flags of `vcr app capabilities add`.
type capabilitiesAddFlags struct {
	capabilities map[string]*bool // by name in manifest.CapabilityNames
	network      manifest.NetworkAPIs
	filename     string
	force        bool
	print        bool
}

var capabilityUsage = map[string]string{
	"voice":        "Add the default Voice capability",
	"messages":     "Add the default Messages capability",
	"rtc":          "Add the default RTC (Real-Time Communication) capability",
	"verify":       "Add the default Verify capability",
	"video":        "Add the default Video capability",
	"vbc":          "Add the default VBC (Vonage Business Communications) capability",
	"meetings":     "Add the default Meetings capability",
	"network_apis": "Add the Network APIs capability (needs --network-application-id and --redirect-uri)",
}

func newCapabilitiesAdd(env *cmdenv.Env) *cobra.Command {
	f := capabilitiesAddFlags{capabilities: map[string]*bool{}}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add default capabilities to the application-capabilities block",
		Long: heredoc.Doc(`Add default capabilities to the application-capabilities block of vcr.yml.

			This command writes the default of each capability you name into the manifest's
			application-capabilities block. Webhook addresses use {{instance_url}}, the URL
			of your deployed instance. The rest of the manifest is kept as written. When the
			manifest has instance.capabilities, it is removed: the block replaces it.

			CAPABILITIES
			  • Voice         (--voice)         - /voice/answer and /voice/event
			  • Messages      (--messages)      - version v1, /messages/inbound and /messages/status
			  • RTC           (--rtc)           - /rtc/event
			  • Verify        (--verify)        - version v2, /verify/status
			  • Video         (--video)         - /video/session-created and five more, all active
			  • VBC           (--vbc)           - an empty mapping
			  • Meetings      (--meetings)      - /meetings/recording-changed, /meetings/room-changed
			                                      and /meetings/session-changed
			  • Network APIs  (--network-apis)  - the values of --network-application-id and --redirect-uri

			Every webhook except Video's is sent with POST. A capability already in the block
			is kept and reported as skipped, unless --force is given.

			NOTE: The manifest is the one vcr deploy would use in the current directory, or
			the one named by --filename. With --print, the resulting block is printed instead
			and no file is written; no manifest is needed.
		`),
		Example: heredoc.Doc(`
			# Add the default Voice and Messages capabilities
			$ vcr app capabilities add --voice --messages
			✓ Added voice to application-capabilities in /path/to/project/vcr.yml
			✓ Added messages to application-capabilities in /path/to/project/vcr.yml

			# Print the block without writing the manifest
			$ vcr app capabilities add --rtc --print

			# Replace a capability that is already configured with its default
			$ vcr app capabilities add --voice --force

			# Add Network APIs
			$ vcr app capabilities add --network-apis --network-application-id my-network-app --redirect-uri https://example.com/callback
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(*cobra.Command, []string) error {
			return runCapabilitiesAdd(env, f)
		},
	}
	flags := cmd.Flags()
	for _, name := range manifest.CapabilityNames {
		f.capabilities[name] = flags.Bool(strings.ReplaceAll(name, "_", "-"), false, capabilityUsage[name])
	}
	flags.StringVar(&f.network.ApplicationID, "network-application-id", "", "Network application ID for --network-apis")
	flags.StringVar(&f.network.RedirectURI, "redirect-uri", "", "Redirect URI for --network-apis")
	flags.StringVarP(&f.filename, "filename", "f", "", "Path to manifest file (default: vcr.yml in project directory)")
	flags.BoolVar(&f.force, "force", false, "Replace capabilities that are already configured with their defaults")
	flags.BoolVar(&f.print, "print", false, "Print the resulting application-capabilities block instead of writing the manifest")
	return cmd
}

func runCapabilitiesAdd(env *cmdenv.Env, f capabilitiesAddFlags) error {
	add, err := requestedCapabilities(f)
	if err != nil {
		return err
	}
	path, src, err := readManifest(f)
	if err != nil {
		return err
	}
	out, res, err := manifest.AddCapabilities(src, add, f.force)
	if err != nil {
		return fmt.Errorf("failed to edit manifest file: %w", err)
	}
	if f.print {
		text, err := encodeBlock(out)
		if err != nil {
			return err
		}
		// Nothing is written, so only the skipped capabilities are reported.
		printResult(env, manifest.Result{Skipped: res.Skipped}, path)
		_, err = fmt.Fprint(env.IO.Out, text)
		return err
	}
	if !bytes.Equal(out, src) {
		if err := manifest.WriteFileAtomic(path, out); err != nil {
			return fmt.Errorf("failed to write manifest file: %w", err)
		}
	}
	printResult(env, res, path)
	return nil
}

// requestedCapabilities checks the flags and returns the default of each capability they name.
func requestedCapabilities(f capabilitiesAddFlags) (map[string]any, error) {
	add := map[string]any{}
	for _, name := range manifest.CapabilityNames {
		if !*f.capabilities[name] {
			continue
		}
		def, err := manifest.DefaultCapability(name, f.network)
		if err != nil {
			return nil, err
		}
		add[name] = def
	}
	network := *f.capabilities["network_apis"]
	switch {
	case len(add) == 0:
		return nil, errNoCapability
	case network && (f.network.ApplicationID == "" || f.network.RedirectURI == ""):
		return nil, errNetworkAPIsValues
	case !network && (f.network.ApplicationID != "" || f.network.RedirectURI != ""):
		return nil, errNetworkAPIsMissing
	}
	return add, nil
}

// readManifest finds the manifest the way deploy does and reads it. With --print and no --filename, a missing
// manifest is an empty one with an empty block.
func readManifest(f capabilitiesAddFlags) (string, []byte, error) {
	dir, err := manifest.ProjectDir("")
	if err != nil {
		return "", nil, err
	}
	path, err := manifest.Find(dir, f.filename)
	if err != nil {
		if f.print && f.filename == "" {
			return "", []byte(capabilitiesKey + ":\n"), nil
		}
		return "", nil, fmt.Errorf("%w (use --print to see the block without a manifest)", err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read manifest file: %w", err)
	}
	return path, src, nil
}

// encodeBlock returns the application-capabilities entry of a manifest as YAML with a two-space indent.
func encodeBlock(src []byte) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return "", err
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != capabilitiesKey {
			continue
		}
		key := *root.Content[i]
		key.HeadComment, key.FootComment = "", ""
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{&key, root.Content[i+1]}}
		if err := enc.Encode(entry); err != nil {
			return "", err
		}
		if err := enc.Close(); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return "", fmt.Errorf("no %s block to print", capabilitiesKey)
}

// printResult prints one line per capability on stderr in CapabilityNames order, then the removed list.
func printResult(env *cmdenv.Env, res manifest.Result, path string) {
	style := env.IO.Style()
	lines := map[string]string{}
	for _, name := range res.Added {
		lines[name] = fmt.Sprintf("%s Added %s to %s in %s", style.SuccessIcon(), name, capabilitiesKey, path)
	}
	for _, name := range res.Replaced {
		lines[name] = fmt.Sprintf("%s Replaced %s in %s in %s", style.SuccessIcon(), name, capabilitiesKey, path)
	}
	for _, name := range res.Skipped {
		lines[name] = fmt.Sprintf("%s %s is already configured, skipped (use --force to replace it)", style.WarningIcon(), name)
	}
	for _, name := range manifest.CapabilityNames {
		if line, ok := lines[name]; ok {
			fmt.Fprintln(env.IO.ErrOut, line)
		}
	}
	if len(res.Removed) > 0 {
		fmt.Fprintf(env.IO.ErrOut, "%s Removed instance.capabilities (%s): %s replaces it\n",
			style.WarningIcon(), strings.Join(res.Removed, ", "), capabilitiesKey)
	}
}
