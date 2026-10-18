package initcmd

import (
	"fmt"
	"regexp"
	"strings"

	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	skipLabel   = "SKIP"
	newAppLabel = "CREATE NEW APP"

	defaultRuntime = "nodejs18"
	debugLanguage  = "debug"
	deprecated     = "deprecated"
	starterPrefix  = "starter project"
)

var (
	projectNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	testRegionPattern  = regexp.MustCompile(`(?i)Test`)
)

func validProjectName(name string) bool { return projectNamePattern.MatchString(name) }

// options are the labels of one picker and the value each label stands for. Labels are built from remote text, so
// they pass through term.Sanitize; the values are what the platform sent. Two entries with the same label resolve
// to the later one, as they always have.
type options[T any] struct {
	labels []string
	values map[string]T
}

func newOptions[T any]() options[T] { return options[T]{values: map[string]T{}} }

func (o *options[T]) add(label string, value T) {
	o.labels = append(o.labels, label)
	o.values[label] = value
}

// runtimeChoice is the runtime a label stands for.
type runtimeChoice struct{ name, language string }

// runtimeOptions offers the runtimes a project can be written for, labelled with their comment when they have one.
func runtimeOptions(runtimes []platform.Runtime) options[runtimeChoice] {
	opts := newOptions[runtimeChoice]()
	for _, r := range runtimes {
		if r.Language == "" || r.Language == debugLanguage || r.Comments == deprecated {
			continue
		}
		label := term.Sanitize(r.Name)
		if r.Comments != "" {
			label = fmt.Sprintf("%s - (%s)", label, term.Sanitize(r.Comments))
		}
		opts.add(label, runtimeChoice{name: r.Name, language: r.Language})
	}
	return opts
}

// runtimeDefault is the label of the default runtime when it is offered without a comment, else "".
func runtimeDefault(opts options[runtimeChoice]) string {
	if _, ok := opts.values[defaultRuntime]; ok {
		return defaultRuntime
	}
	return ""
}

// regionOptions are the pickable regions; their values are the aliases.
type regionOptions struct {
	options[string]
	labelByAlias map[string]string
}

// newRegionOptions offers every region except the test ones.
func newRegionOptions(regions []platform.Region) regionOptions {
	opts := regionOptions{options: newOptions[string](), labelByAlias: map[string]string{}}
	for _, r := range regions {
		if testRegionPattern.MatchString(r.Name) || testRegionPattern.MatchString(r.Alias) {
			continue
		}
		label := fmt.Sprintf("%s - (%s)", term.Sanitize(r.Name), term.Sanitize(r.Alias))
		opts.add(label, r.Alias)
		opts.labelByAlias[r.Alias] = label
	}
	return opts
}

// defaultLabel is the label of the region with the alias, or "" when it is not offered.
func (o regionOptions) defaultLabel(alias string) string { return o.labelByAlias[alias] }

// appOptions offers SKIP, CREATE NEW APP and the applications, whose values are their ids. The first two stand for
// no id.
func appOptions(apps []platform.Application) options[string] {
	opts := newOptions[string]()
	opts.labels = append(opts.labels, skipLabel, newAppLabel)
	for _, a := range apps {
		opts.add(fmt.Sprintf("%s - (%s)", term.Sanitize(a.Name), term.Sanitize(a.ID)), a.ID)
	}
	return opts
}

// templatesFor keeps the templates written for language, the starter projects first, each group in the order the
// platform listed them.
func templatesFor(language string, templates []platform.Template) []platform.Template {
	var starters, others []platform.Template
	for _, t := range templates {
		if strings.ToLower(t.ProgrammingLanguage) != language {
			continue
		}
		if strings.HasPrefix(strings.ToLower(t.Name), starterPrefix) {
			starters = append(starters, t)
		} else {
			others = append(others, t)
		}
	}
	return append(starters, others...)
}

// templateOptions offers SKIP and the templates, labelled with their names.
func templateOptions(templates []platform.Template) options[platform.Template] {
	opts := newOptions[platform.Template]()
	opts.labels = append(opts.labels, skipLabel)
	for _, t := range templates {
		opts.add(term.Sanitize(t.Name), t)
	}
	return opts
}
