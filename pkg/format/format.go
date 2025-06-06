package format

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"

	"vonage-cloud-runtime-cli/pkg/cmdutil"

	"vonage-cloud-runtime-cli/pkg/api"
)

const SkipValue = "SKIP"
const NewAppValue = "CREATE NEW APP"

type TemplateOptions struct {
	Labels   []string
	IDLookup map[string]string
}

func GetTemplateOptions(templateNames []api.Product) TemplateOptions {
	options := TemplateOptions{
		Labels:   make([]string, 0),
		IDLookup: make(map[string]string),
	}
	options.Labels = append(options.Labels, SkipValue)
	for _, r := range templateNames {
		options.Labels = append(options.Labels, r.Name)
		options.IDLookup[r.Name] = r.ID
	}
	return options
}

var testRegex = regexp.MustCompile(`(?i)Test`)

type RegionOptions struct {
	Labels      []string
	Lookup      map[string]string
	AliasLookup map[string]string
}

func GetRegionOptions(regions []api.Region) RegionOptions {
	options := RegionOptions{
		Labels:      make([]string, 0),
		Lookup:      make(map[string]string),
		AliasLookup: make(map[string]string),
	}

	for _, r := range regions {
		if testRegex.MatchString(r.Name) || testRegex.MatchString(r.Alias) {
			continue
		}
		label := fmt.Sprintf("%s - (%s)", r.Name, r.Alias)
		options.Labels = append(options.Labels, label)
		options.Lookup[r.Alias] = label
		options.AliasLookup[label] = r.Alias
	}
	return options
}

type RuntimeOptions struct {
	Labels                []string
	Lookup                map[string]string
	RuntimeLookup         map[string]string
	ProgrammingLangLookup map[string]string
}

func GetRuntimeOptions(runtimes []api.Runtime) RuntimeOptions {
	options := RuntimeOptions{
		Labels:                make([]string, 0),
		Lookup:                make(map[string]string),
		RuntimeLookup:         make(map[string]string),
		ProgrammingLangLookup: make(map[string]string),
	}

	for _, r := range runtimes {
		if r.Language != "debug" && r.Language != "" && r.Comments != "deprecated" {
			if r.Comments != "" {
				label := fmt.Sprintf("%s - (%s)", r.Name, r.Comments)
				options.Labels = append(options.Labels, label)
				options.Lookup[r.Name] = label
				options.RuntimeLookup[label] = r.Name
				options.ProgrammingLangLookup[label] = r.Language
				continue
			}
			options.Labels = append(options.Labels, r.Name)
			options.Lookup[r.Name] = r.Name
			options.RuntimeLookup[r.Name] = r.Name
			options.ProgrammingLangLookup[r.Name] = r.Language
		}
	}
	return options
}

type AppOptions struct {
	Labels   []string
	Lookup   map[string]string
	IDLookup map[string]string
}

func GetAppOptions(apps []api.ApplicationListItem) AppOptions {
	options := AppOptions{
		Labels:   make([]string, 0),
		Lookup:   map[string]string{},
		IDLookup: make(map[string]string),
	}
	options.Labels = append(options.Labels, SkipValue)
	options.Labels = append(options.Labels, NewAppValue)
	for _, r := range apps {
		label := fmt.Sprintf("%s - (%s)", r.Name, r.ID)
		options.Labels = append(options.Labels, label)
		options.Lookup[r.ID] = label
		options.IDLookup[label] = r.ID
	}
	return options
}

func ParseCapabilities(caps []string) (api.Capabilities, error) {
	parsedCaps := api.Capabilities{}
	for _, c := range caps {
		switch {
		case strings.HasPrefix(c, "messages"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Messages = v
		case strings.HasPrefix(c, "messaging"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Messages = v
		case strings.HasPrefix(c, "voice"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Voice = v
		case strings.HasPrefix(c, "rtc"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.RTC = v
		case strings.HasPrefix(c, "video"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Video = v
		case strings.HasPrefix(c, "verify"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Verify = v
		case strings.HasPrefix(c, "network"):
			v, err := parseCapVersion(c)
			if err != nil {
				return api.Capabilities{}, err
			}
			parsedCaps.Network = v
		}
	}
	return parsedCaps, nil
}

func parseCapVersion(capability string) (string, error) {
	if capability == "messages" || capability == "messaging" {
		return "v0.1", nil
	}
	if capability == "voice" || capability == "rtc" || capability == "video" || capability == "network" {
		return "v0", nil
	}
	parts := strings.Split(capability, "-")
	const expectedParts = 2
	if len(parts) != expectedParts {
		return "", fmt.Errorf("invalid capability - make sure update is referenced correctly")
	}
	return parts[1], nil
}

var versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func PrintUpdateMessage(out *iostreams.IOStreams, version string, updateMessageChan chan string) {
	c := out.ColorScheme()
	const updateCheckTimeout = 500 * time.Millisecond
	select {
	case rel, ok := <-updateMessageChan:
		if ok {
			if rel != "" {
				if !versionRegex.MatchString(rel) {
					return
				}
				version = strings.TrimPrefix(version, "v")
				if version == "dev" {
					version = "0.0.1"
				}
				fmt.Fprintf(out.Out, "\n\n%s %s → %s\n",
					c.Yellow("A new release of vcr is available:"),
					c.Cyan(strings.TrimPrefix(version, "v")),
					c.Cyan(strings.TrimPrefix(rel, "v")))

				fmt.Fprintf(out.Out, "To upgrade, run: %s\n", "vcr upgrade")
				return
			}
			return
		}
		return
	case <-time.After(updateCheckTimeout):
		close(updateMessageChan)
		return
	}
}

func PrintAPIError(out *iostreams.IOStreams, err error, httpErr *api.Error) string {
	c := out.ColorScheme()
	mainErrMsg, err := extractFinalErrorMessage(err)
	if err != nil {
		mainErrMsg = err.Error()
	}
	var mainIssue, httpStatus, errorCode, detailedMessage, traceID, containerLogs string
	mainIssue = fmt.Sprintf("%s Details:", c.Red(cmdutil.InfoIcon))
	httpStatus = fmt.Sprintf("- HTTP Status : %s", strconv.Itoa(httpErr.HTTPStatusCode))
	errorCode = fmt.Sprintf("- Error Code  : %s", strconv.Itoa(httpErr.ServerCode))
	detailedMessage = fmt.Sprintf("- Message     : %s", httpErr.Message)
	traceID = fmt.Sprintf("- Trace ID    : %s", httpErr.TraceID)
	containerLogs = fmt.Sprintf("%s App logs captured before failure:\n%s", c.Red(cmdutil.InfoIcon), httpErr.ContainerLogs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error Encountered: %s\n\n", mainErrMsg))
	sb.WriteString(fmt.Sprintf("%s\n", mainIssue))
	if httpErr.HTTPStatusCode != 0 {
		sb.WriteString(fmt.Sprintf("  %s\n", httpStatus))
	}
	if httpErr.ServerCode != 0 {
		sb.WriteString(fmt.Sprintf("  %s\n", errorCode))
	}
	if httpErr.Message != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", detailedMessage))
	}
	if httpErr.TraceID != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", traceID))
	}
	if httpErr.ContainerLogs != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", containerLogs))
	}
	sb.WriteString("\nPlease refer to the documentation or contact support for further assistance.")
	return sb.String()
}

var errorRegex = regexp.MustCompile("^[^:]+")

func extractFinalErrorMessage(err error) (string, error) {
	matches := errorRegex.FindStringSubmatch(err.Error())
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", errors.New("no match found")
}
