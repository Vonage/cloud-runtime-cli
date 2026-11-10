package initcmd

import (
	"context"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/scaffold"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The methods below are what the lookups of init show on a terminal. Each makes the call of the classic method beside
// it, resolves before the next question is asked and returns what that method returns; only the spinner and the
// finished line differ. A step that fails is erased with Stop and not failed: the error is returned as it is and
// printed once, by the renderer of errors.

// lookup runs call under a step that resolves into done when the call succeeds.
func lookup[T any](s *session, label, done string, call func() (T, error)) (T, error) {
	step := s.io.StartStep(label)
	value, err := call()
	if err != nil {
		step.Stop()
		return value, err
	}
	step.OK(done)
	return value, nil
}

func (s *session) retrieveRuntimesRich(ctx context.Context) ([]platform.Runtime, error) {
	return lookup(s, "Retrieving runtimes", "Runtimes retrieved", func() ([]platform.Runtime, error) {
		return s.api.ListRuntimes(ctx)
	})
}

func (s *session) retrieveRegionsRich(ctx context.Context) ([]platform.Region, error) {
	return lookup(s, "Retrieving regions", "Regions retrieved", func() ([]platform.Region, error) {
		return s.api.ListRegions(ctx)
	})
}

func (s *session) retrieveApplicationsRich(ctx context.Context) ([]platform.Application, error) {
	return lookup(s, "Retrieving applications", "Applications retrieved", func() ([]platform.Application, error) {
		return s.api.ListApplications(ctx, "")
	})
}

func (s *session) submitApplicationRich(ctx context.Context, name string) (platform.CreatedApplication, error) {
	done := "Application created  " + s.style.Gray(term.Sanitize(name))
	return lookup(s, "Creating application", done, func() (platform.CreatedApplication, error) {
		return s.api.CreateApplication(ctx, name, false, false, false)
	})
}

func (s *session) retrieveTemplatesRich(ctx context.Context) ([]platform.Template, error) {
	return lookup(s, "Retrieving templates", "Templates retrieved", func() ([]platform.Template, error) {
		return s.api.ListTemplates(ctx)
	})
}

func (s *session) downloadTemplateRich(ctx context.Context, template platform.Template) ([]byte, error) {
	return lookup(s, "Downloading template", "Template downloaded", func() ([]byte, error) {
		return s.api.DownloadTemplate(ctx, template.ID, template.ActiveVersionID)
	})
}

func (s *session) unpackTemplateRich(archive []byte) error {
	step := s.io.StartStep("Unpacking template")
	if err := s.proj.Extract(archive, scaffold.DefaultLimits); err != nil {
		step.Stop()
		return err
	}
	step.OK("Template unpacked")
	return nil
}

// finishRich names the directory the project was made in and what to do next.
func (s *session) finishRich() {
	fmt.Fprintf(s.io.Out, "%s %s\n", s.style.SuccessIcon(), s.style.Bold("Project created in "+s.dir))
	fmt.Fprintln(s.io.Out, s.style.Gray(nextStep(s.dir)))
}
