package debugsession

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
)

// debugVars are the values behind the fixed variables of the local application's environment.
type debugVars struct {
	ServiceName string
	InstanceID  string // an empty id leaves INSTANCE_ID out
	PrivateKey  string
	PublicURL   string // the http(s) host URL printed as "Application Host"
	AppID       string
	APIKey      string
	APISecret   string
	ProjectDir  string // absolute
	Region      platform.Region
	AppPort     int
}

// debugEnvironment returns base, then the manifest variables, then the fixed variables. A process started with it
// sees the last value of a name that appears twice, so a fixed variable beats a manifest variable of the same name.
// The CLI's own environment is never touched.
func debugEnvironment(base, manifestVars []string, v debugVars) []string {
	port := strconv.Itoa(v.AppPort)
	fixed := []string{
		"DEBUG=true",
		"INSTANCE_SERVICE_NAME=" + v.ServiceName,
		"API_ACCOUNT_ID=" + v.APIKey,
		"API_APPLICATION_ID=" + v.AppID,
		"API_ACCOUNT_SECRET=" + v.APISecret,
		"PRIVATE_KEY=" + v.PrivateKey,
		"CODE_DIR=" + v.ProjectDir,
		"ENDPOINT_URL_SCHEME=" + v.Region.EndpointURLScheme,
		"DEBUGGER_URL_SCHEME=" + v.Region.DebuggerURLScheme,
		"REGION=" + v.Region.Alias,
		"NERU_APP_PORT=" + port,
		"VCR_DEBUG=true",
		"VCR_INSTANCE_SERVICE_NAME=" + v.ServiceName,
		"VCR_INSTANCE_PUBLIC_URL=" + v.PublicURL,
		"VCR_API_ACCOUNT_ID=" + v.APIKey,
		"VCR_API_ACCOUNT_SECRET=" + v.APISecret,
		"VCR_API_APPLICATION_ID=" + v.AppID,
		"VCR_PRIVATE_KEY=" + v.PrivateKey,
		"VCR_CODE_DIR=" + v.ProjectDir,
		"VCR_ENDPOINT_URL_SCHEME=" + v.Region.EndpointURLScheme,
		"VCR_DEBUGGER_URL_SCHEME=" + v.Region.DebuggerURLScheme,
		"VCR_REGION=" + v.Region.Alias,
		"VCR_PORT=" + port,
		"FORCE_COLOR=1",
	}
	if v.InstanceID != "" {
		fixed = append(fixed, "INSTANCE_ID="+v.InstanceID)
	}
	out := make([]string, 0, len(base)+len(manifestVars)+len(fixed))
	out = append(out, base...)
	out = append(out, manifestVars...)
	return append(out, fixed...)
}

// manifestEnvironment returns the NAME=value entries of the manifest's debug environment, or of its instance
// environment when the debug one is empty (the two are never merged). usedInstance reports that the instance list was
// the one used, also when it failed, so the caller can name the list in the error (see wrapEnvError).
// A secret entry takes its value from the CLI's own environment, read through lookup.
func manifestEnvironment(m *manifest.Manifest, lookup func(string) string) (vars []string, usedInstance bool, err error) {
	list := m.Debug.Environment
	if len(list) == 0 {
		list = m.Instance.Environment
		usedInstance = len(list) != 0
	}
	for _, e := range list {
		entry, entryErr := environmentEntry(e, lookup)
		if entryErr != nil {
			return nil, usedInstance, entryErr
		}
		vars = append(vars, entry)
	}
	return vars, usedInstance, nil
}

func environmentEntry(e manifest.Env, lookup func(string) string) (string, error) {
	value := e.Value
	if e.Secret != "" {
		value = lookup(e.Secret)
		if value == "" {
			return "", fmt.Errorf("environment variable %q must be exported locally in debug mode", e.Secret)
		}
	}
	if err := checkVariable(e.Name, value); err != nil {
		return "", fmt.Errorf("%q: %w", e.Name, err)
	}
	return e.Name + "=" + value, nil
}

// checkVariable refuses what os.Setenv refuses, with its error text: earlier releases set the variables on the CLI's
// own process, and users see that message.
func checkVariable(name, value string) error {
	if name == "" || strings.ContainsAny(name, "=\x00") || strings.Contains(value, "\x00") {
		return os.NewSyscallError("setenv", syscall.EINVAL)
	}
	return nil
}

// wrapEnvError names the manifest list that failed.
func wrapEnvError(usedInstance bool, err error) error {
	list := "debug"
	if usedInstance {
		list = "instance"
	}
	return fmt.Errorf("failed to inject %s environment variables: %w", list, err)
}
