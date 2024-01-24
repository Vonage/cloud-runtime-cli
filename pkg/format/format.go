package format

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"vonage-cloud-runtime-cli/pkg/api"
)

type TemplateOptions struct {
	Labels     []string
	NameLookup map[string]string
}

func GetTemplateOptions(templateNames []string, prefix string) TemplateOptions {
	options := TemplateOptions{
		Labels:     make([]string, 0),
		NameLookup: make(map[string]string),
	}
	options.Labels = append(options.Labels, "SKIP")
	for _, r := range templateNames {
		label := strings.TrimPrefix(r, prefix)
		label = strings.TrimSuffix(label, ".zip")
		options.Labels = append(options.Labels, label)
		options.NameLookup[label] = r
	}
	return options
}

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
		label := fmt.Sprintf("%s - (%s)", r.Name, r.Alias)
		options.Labels = append(options.Labels, label)
		options.Lookup[r.Alias] = label
		options.AliasLookup[label] = r.Alias
	}
	return options
}

type RuntimeOptions struct {
	Labels        []string
	Lookup        map[string]string
	RuntimeLookup map[string]string
}

func GetRuntimeOptions(runtimes []api.Runtime) RuntimeOptions {
	options := RuntimeOptions{
		Labels:        make([]string, 0),
		Lookup:        make(map[string]string),
		RuntimeLookup: make(map[string]string),
	}

	for _, r := range runtimes {
		if r.Language != "debug" {
			if r.Comments != "" {
				label := fmt.Sprintf("%s - (%s)", r.Name, r.Comments)
				options.Labels = append(options.Labels, label)
				options.Lookup[r.Name] = label
				options.RuntimeLookup[label] = r.Name
				continue
			}
			options.Labels = append(options.Labels, r.Name)
			options.Lookup[r.Name] = r.Name
			options.RuntimeLookup[r.Name] = r.Name
		}
	}
	return options
}

type AppOptions struct {
	Labels   []string
	Lookup   map[string]string
	IdLookup map[string]string
}

func GetAppOptions(apps []api.ApplicationListItem) AppOptions {
	options := AppOptions{
		Labels:   make([]string, 0),
		Lookup:   map[string]string{},
		IdLookup: make(map[string]string),
	}
	options.Labels = append(options.Labels, "SKIP")
	for _, r := range apps {
		label := fmt.Sprintf("%s - (%s)", r.Name, r.ID)
		options.Labels = append(options.Labels, label)
		options.Lookup[r.ID] = label
		options.IdLookup[label] = r.ID
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
		}
	}
	return parsedCaps, nil
}

func parseCapVersion(cap string) (string, error) {
	if cap == "messages" || cap == "messaging" {
		return "v0.1", nil
	}
	if cap == "voice" || cap == "rtc" {
		return "v0", nil
	}
	parts := strings.Split(cap, "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid capability - make sure update is referenced correctly")
	}
	return parts[1], nil
}

var versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func PrintUpdateMessage(out *iostreams.IOStreams, version string, updateMessageChan chan string) {
	c := out.ColorScheme()
	select {
	case rel, ok := <-updateMessageChan:
		if ok {
			if rel != "" {
				if !versionRegex.MatchString(rel) {
					fmt.Fprintf(out.ErrOut, "\n\n%s\n", rel)
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
	case <-time.After(500 * time.Millisecond):
		close(updateMessageChan)
		return
	}
}

func PrintAPIError(err error, httpErr *api.Error) string {
	mainErrMsg, err := extractFinalErrorMessage(err)
	if err != nil {
		mainErrMsg = err.Error()
	}
	var mainIssue, httpStatus, errorCode, detailedMessage, traceID, containerLogs string
	mainIssue = fmt.Sprintf("Main issue : %s", mainErrMsg)
	httpStatus = fmt.Sprintf("HTTP status : %s", strconv.Itoa(httpErr.HTTPStatusCode))
	errorCode = fmt.Sprintf("Error code : %s", strconv.Itoa(httpErr.ServerCode))
	detailedMessage = fmt.Sprintf("Detailed message : %s", httpErr.Message)
	traceID = fmt.Sprintf("Trace ID : %s", httpErr.TraceId)
	containerLogs = fmt.Sprintf("Container logs : %s", httpErr.ContainerLogs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("API Error Encountered:\n 	%s\n 	%s\n", mainIssue, httpStatus))
	if httpErr.ServerCode != 0 {
		sb.WriteString(fmt.Sprintf(" 	%s\n", errorCode))
	}
	if httpErr.Message != "" {
		sb.WriteString(fmt.Sprintf(" 	%s\n", detailedMessage))
	}
	if httpErr.TraceId != "" {
		sb.WriteString(fmt.Sprintf(" 	%s\n", traceID))
	}
	if httpErr.ContainerLogs != "" {
		sb.WriteString(fmt.Sprintf(" 	%s\n", containerLogs))
	}
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
