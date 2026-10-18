package initcmd

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/scaffold"
	"vonage-cloud-runtime-cli/internal/term"
)

// applyTemplate lets the user pick a template for the runtime and unpacks it into the project. It returns the
// manifest to write and the name of the file to write it under: the template's own manifest with the answers laid
// over it, or the answers alone when no template is used.
func (s *session) applyTemplate(ctx context.Context, chosen answers) (*manifest.Manifest, string, error) {
	answered := chosen.manifest()
	template, picked, err := s.askTemplate(ctx, chosen.runtime)
	if err != nil || !picked {
		return answered, manifestName, err
	}
	if err = s.unpack(ctx, template); err != nil {
		return nil, "", err
	}
	return s.layOverTemplateManifest(answered)
}

// askTemplate offers the templates written for the runtime's language. picked is false when none exists or the
// user skips.
func (s *session) askTemplate(ctx context.Context, rt runtimeChoice) (platform.Template, bool, error) {
	var (
		all []platform.Template
		err error
	)
	s.waiting(" Retrieving product templates... ", func() { all, err = s.api.ListTemplates(ctx) })
	if err != nil {
		return platform.Template{}, false, fmt.Errorf("failed to list product templates: %w", err)
	}
	matching := templatesFor(rt.language, all)
	if len(matching) == 0 {
		fmt.Fprintf(s.io.ErrOut, "%s No product templates available for the selected runtime %q\n",
			s.style.WarningIcon(), rt.name)
		return platform.Template{}, false, nil
	}

	opts := templateOptions(matching)
	label, err := s.prompt.Select(fmt.Sprintf("Select a product template for runtime %s: ", term.Sanitize(rt.name)), opts.labels, "")
	if err != nil {
		return platform.Template{}, false, fmt.Errorf("failed to ask user to select a product template for runtime %s: %w", rt.name, err)
	}
	if label == skipLabel {
		return platform.Template{}, false, nil
	}
	template := opts.values[label]
	if template.ActiveVersionID == "" {
		return platform.Template{}, false, fmt.Errorf("failed to get the active product template version: %w", platform.ErrNotFound)
	}
	return template, true, nil
}

// unpack downloads the template's active version and extracts it into the project.
func (s *session) unpack(ctx context.Context, template platform.Template) error {
	var (
		archive []byte
		err     error
	)
	s.waiting(" Downloading template files... ", func() {
		archive, err = s.api.DownloadTemplate(ctx, template.ID, template.ActiveVersionID)
	})
	if err != nil {
		return fmt.Errorf("failed to download template files: %w", err)
	}
	s.waiting(" Uncompressing template files... ", func() { err = s.proj.Extract(archive, scaffold.DefaultLimits) })
	if err != nil {
		return fmt.Errorf("failed to uncompress template files: %w", err)
	}
	return nil
}

// layOverTemplateManifest reads the manifest the template brought. The answers win over its six values and every
// other key of it survives. A manifest that cannot be parsed is reported and the answers are written in its place.
func (s *session) layOverTemplateManifest(answered *manifest.Manifest) (*manifest.Manifest, string, error) {
	fromTemplate, name, found, err := s.proj.ReadManifest()
	switch {
	case !found && err != nil:
		return nil, "", fmt.Errorf("failed to find template manifest file: %w", err)
	case !found:
		return nil, "", fmt.Errorf("failed to find template manifest file: %w", errNoManifest)
	case err != nil:
		fmt.Fprintf(s.io.ErrOut, "%s Failed to parse template manifest file due to %s.\n",
			s.style.WarningIcon(), term.Sanitize(err.Error()))
		return answered, name, nil
	}
	return manifest.Merge(fromTemplate, answered), name, nil
}
