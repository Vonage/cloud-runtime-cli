package initcmd

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

// appQuestion words the two application questions.
type appQuestion struct {
	pick    string // the picker's message
	newName string // the message that asks for the name of a new application
	failure string // what an error of the question is wrapped with
}

var (
	deploymentApp = appQuestion{
		pick:    "Select your Vonage application ID for deployment:",
		newName: "Enter your new Vonage application name for deployment:",
		failure: "failed to ask instance app id",
	}
	debugApp = appQuestion{
		pick:    "Select your Vonage application ID for debug:",
		newName: "Enter your new Vonage application name for debug:",
		failure: "failed to ask debug app id",
	}
)

// askApplication returns the id of the application the user picks or creates, or the placeholder when the user
// skips. The list is fetched again for each question, so an application made for the first shows up in the second.
func (s *session) askApplication(ctx context.Context, q appQuestion) (string, error) {
	id, err := s.chooseApplication(ctx, q)
	if err != nil {
		return "", fmt.Errorf("%s: %w", q.failure, err)
	}
	return id, nil
}

func (s *session) chooseApplication(ctx context.Context, q appQuestion) (string, error) {
	var (
		apps []platform.Application
		err  error
	)
	s.waiting(" Retrieving app list... ", func() { apps, err = s.api.ListApplications(ctx, "") })
	if err != nil {
		return "", err
	}
	opts := appOptions(apps)
	label, err := s.prompt.Select(q.pick, opts.labels, "")
	if err != nil {
		return "", err
	}
	switch label {
	case skipLabel:
		return placeholderApp, nil
	case newAppLabel:
		return s.createApplication(ctx, q.newName)
	default:
		return opts.values[label], nil
	}
}

// createApplication asks for a name and creates the application, asking again after a refusal.
func (s *session) createApplication(ctx context.Context, question string) (string, error) {
	for {
		name, err := s.prompt.Input(question, "")
		if err != nil {
			return "", err
		}
		var (
			created platform.CreatedApplication
			failed  error
		)
		s.waiting(fmt.Sprintf(" Creating Application %q...", name), func() {
			created, failed = s.api.CreateApplication(ctx, name, false, false, false)
		})
		if failed == nil {
			return created.ApplicationID, nil
		}
		fmt.Fprintf(s.io.ErrOut, "%s %s\n", s.style.FailureIcon(), term.Sanitize(failed.Error()))
	}
}
