package manifest

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Read parses a manifest file.
func Read(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse is the non-strict YAML parse Read uses, for bytes read elsewhere (scaffold reads through an os.Root).
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
