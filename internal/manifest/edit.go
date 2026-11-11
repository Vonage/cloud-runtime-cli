package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const blockKey = "application-capabilities"

// Result says what AddCapabilities did, each list in CapabilityNames order except Removed, which holds the
// instance.capabilities entries as written.
type Result struct {
	Added, Replaced, Skipped, Removed []string
}

// AddCapabilities sets the capabilities in add (keyed by a name in CapabilityNames) in the manifest's
// application-capabilities block, creating the block at the end of the file if needed. A capability already in the
// block is skipped unless force is set, in which case it is replaced in place. Once the block holds anything,
// instance.capabilities is removed. Everything else in src keeps its comments, order and quoting.
func AddCapabilities(src []byte, add map[string]any, force bool) ([]byte, Result, error) {
	var res Result
	if err := checkNames(add); err != nil {
		return nil, res, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, res, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, res, errors.New("the manifest is not a YAML mapping")
	}
	root := doc.Content[0]
	block, err := capabilitiesBlock(root)
	if err != nil {
		return nil, res, err
	}
	for _, name := range CapabilityNames {
		value, ok := add[name]
		if !ok {
			continue
		}
		if err := setCapability(block, name, value, force, &res); err != nil {
			return nil, res, err
		}
	}
	if len(block.Content) > 0 {
		res.Removed = removeInstanceCapabilities(root)
	}
	out, err := encode(&doc)
	return out, res, err
}

func checkNames(add map[string]any) error {
	for name := range add {
		if _, ok := defaults[name]; !ok {
			return fmt.Errorf("unknown capability %q", name)
		}
	}
	return nil
}

// capabilitiesBlock returns the block's mapping, appending an empty one to root when the key is missing or empty.
func capabilitiesBlock(root *yaml.Node) (*yaml.Node, error) {
	i := keyIndex(root, blockKey)
	if i < 0 {
		block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, scalar(blockKey), block)
		return block, nil
	}
	value := root.Content[i+1]
	switch {
	case value.Kind == yaml.MappingNode:
		return value, nil
	case value.Kind == yaml.ScalarNode && value.Tag == "!!null":
		block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content[i+1] = block
		return block, nil
	default:
		return nil, fmt.Errorf("%s in the manifest is not a mapping", blockKey)
	}
}

func setCapability(block *yaml.Node, name string, value any, force bool, res *Result) error {
	i := keyIndex(block, name)
	if i >= 0 && !force {
		res.Skipped = append(res.Skipped, name)
		return nil
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("failed to encode %s: %w", name, err)
	}
	if i >= 0 {
		block.Content[i+1] = &node
		res.Replaced = append(res.Replaced, name)
		return nil
	}
	block.Content = append(block.Content, scalar(name), &node)
	res.Added = append(res.Added, name)
	return nil
}

// removeInstanceCapabilities deletes instance.capabilities and returns its scalar entries in file order.
func removeInstanceCapabilities(root *yaml.Node) []string {
	i := keyIndex(root, "instance")
	if i < 0 || root.Content[i+1].Kind != yaml.MappingNode {
		return nil
	}
	instance := root.Content[i+1]
	j := keyIndex(instance, "capabilities")
	if j < 0 {
		return nil
	}
	var removed []string
	for _, entry := range instance.Content[j+1].Content {
		if entry.Kind == yaml.ScalarNode {
			removed = append(removed, entry.Value)
		}
	}
	instance.Content = append(instance.Content[:j], instance.Content[j+2:]...)
	return removed
}

// keyIndex returns the index of key's key node in a mapping's Content, or -1.
func keyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func encode(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteFileAtomic replaces an existing file with data through a temporary file in the same directory, keeping the
// file's mode. The temporary file never outlives a failure.
func WriteFileAtomic(path string, data []byte) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vcr-manifest-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
