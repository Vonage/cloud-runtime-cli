package manifest

import (
	"slices"

	"gopkg.in/yaml.v3"
)

// Merge returns a new manifest: base with exactly the six values v2.6.1's Merge copies from override, unconditionally
// (project name, instance name, instance application id, debug application id, region, runtime). Inputs are not
// modified.
func Merge(base, override *Manifest) *Manifest {
	merged := cloneManifest(base)
	merged.Project.Name = override.Project.Name
	merged.Instance.Name = override.Instance.Name
	merged.Instance.ApplicationID = override.Instance.ApplicationID
	merged.Debug.ApplicationID = override.Debug.ApplicationID
	merged.Instance.Region = override.Instance.Region
	merged.Instance.Runtime = override.Instance.Runtime
	return merged
}

// cloneManifest copies every slice and pointer of m, so that nothing written through the copy reaches m.
func cloneManifest(m *Manifest) *Manifest {
	clone := *m
	clone.Instance.Environment = slices.Clone(m.Instance.Environment)
	clone.Instance.Capabilities = slices.Clone(m.Instance.Capabilities)
	clone.Instance.Entrypoint = slices.Clone(m.Instance.Entrypoint)
	clone.Instance.Domains = slices.Clone(m.Instance.Domains)
	if m.Instance.Security != nil {
		security := *m.Instance.Security
		security.Override = slices.Clone(security.Override)
		clone.Instance.Security = &security
	}
	clone.Debug.Environment = slices.Clone(m.Debug.Environment)
	clone.Debug.Entrypoint = slices.Clone(m.Debug.Entrypoint)
	clone.ApplicationCapabilities = cloneBlock(m.ApplicationCapabilities)
	return &clone
}

// Marshal returns the YAML bytes v2.6.1 writes for m.
func Marshal(m *Manifest) ([]byte, error) {
	return yaml.Marshal(m)
}
