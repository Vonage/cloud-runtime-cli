package deploy

import (
	"strings"

	stages "github.com/Vonage/cloud-runtime-cli/internal/deploy"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

// Where the region of a deploy came from.
const (
	fromFlag     = "flag"
	fromManifest = "manifest"
	fromConfig   = "config"
	fromNowhere  = "none"
)

// effectiveRegion is the first region that is set of the --region flag, the manifest's instance.region and the
// config file's default_region, with the place it came from. Only that one is looked up, so a default region the
// platform does not know matters only when nothing above it is set.
func effectiveRegion(flagRegion string, read *manifest.Manifest, configRegion string) (region, source string) {
	switch {
	case flagRegion != "":
		return flagRegion, fromFlag
	case read.Instance.Region != "":
		return read.Instance.Region, fromManifest
	case configRegion != "":
		return configRegion, fromConfig
	}
	return "", fromNowhere
}

// buildPlan merges the flags into a copy of the manifest that was read: a flag that is set replaces the manifest's
// value, and --capabilities replaces the whole list, split on commas as typed. Nothing is required here; the stages
// report what is missing when they reach it.
func buildPlan(f flagValues, read *manifest.Manifest, dir, region string) stages.Plan {
	merged := *read
	merged.Project.Name = firstSet(f.projectName, read.Project.Name)
	merged.Instance.Name = firstSet(f.instanceName, read.Instance.Name)
	merged.Instance.Runtime = firstSet(f.runtime, read.Instance.Runtime)
	merged.Instance.ApplicationID = firstSet(f.appID, read.Instance.ApplicationID)
	if caps := flagCapabilities(f.capabilities); caps != nil {
		merged.Instance.Capabilities = caps
	}
	return stages.Plan{
		ProjectDir:   dir,
		Manifest:     &merged,
		ProjectName:  merged.Project.Name,
		InstanceName: merged.Instance.Name,
		Runtime:      merged.Instance.Runtime,
		Region:       region,
		AppID:        merged.Instance.ApplicationID,
		Capabilities: merged.Instance.Capabilities,
		TgzPath:      f.tgz,
	}
}

// flagCapabilities is the --capabilities value split on commas as typed, or nil when the flag is empty.
func flagCapabilities(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// firstSet returns preferred unless it is empty.
func firstSet(preferred, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}
