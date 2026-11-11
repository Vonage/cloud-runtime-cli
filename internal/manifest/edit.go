package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const blockKey = "application-capabilities"

// Result says what AddCapabilities did, each list in CapabilityNames order except Removed, which holds the
// instance.capabilities entries as written.
type Result struct {
	Added, Replaced, Skipped, Removed []string
}

// splice replaces lines [from, to) of the source with text.
type splice struct {
	from, to int
	text     string
}

// AddCapabilities sets the capabilities in add (keyed by a name in CapabilityNames) in the manifest's
// application-capabilities block, creating the block at the end of the file if needed. A capability already in the
// block is skipped unless force is set, in which case it is replaced in place. Once the block holds anything,
// instance.capabilities is removed. Only the block is re-encoded, with the file's indent; it and the
// instance.capabilities lines are spliced into src by line, so every other byte is kept. A file with more than one
// YAML document is refused, since only the first is read.
func AddCapabilities(src []byte, add map[string]any, force bool) ([]byte, Result, error) {
	var res Result
	if err := checkNames(add); err != nil {
		return nil, res, err
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(src))
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, res, err
	}
	// A second Decode that finds anything, even a syntax error after a "..." marker, means a second document.
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, res, errors.New("manifest holds more than one YAML document: " +
			blockKey + " would not be read from the second")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, res, errors.New("the manifest is not a YAML mapping")
	}
	root := doc.Content[0]
	if root.Style&yaml.FlowStyle != 0 {
		return nil, res, errors.New("the manifest is a flow mapping ({...}); write it in block style to edit it")
	}
	lines := strings.SplitAfter(string(src), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	indent := detectIndent(root)

	i, block, empty, err := capabilitiesBlock(root)
	if err != nil {
		return nil, res, err
	}
	changed := false
	for _, name := range CapabilityNames {
		value, ok := add[name]
		if !ok {
			continue
		}
		c, err := setCapability(block, name, value, force, &res)
		if err != nil {
			return nil, res, err
		}
		changed = changed || c
	}

	var edits []splice
	if changed {
		key := scalar(blockKey)
		from, to := len(lines), len(lines)
		if i >= 0 {
			key = root.Content[i]
			from, to = entryRange(root, i, len(lines), lines)
		}
		if empty && key.LineComment == "" {
			k := *key
			k.LineComment = root.Content[i+1].LineComment
			key = &k
		}
		text, err := encodeEntry(key, block, indent)
		if err != nil {
			return nil, res, err
		}
		if empty {
			text = keepCommentLines(text, lines[from+1:to])
		}
		edits = append(edits, splice{from, to, text})
	}
	if len(block.Content) > 0 {
		removed, edit, err := removeInstanceCapabilities(root, lines, indent)
		if err != nil {
			return nil, res, err
		}
		if edit != nil {
			res.Removed = removed
			edits = append(edits, *edit)
		}
	}
	return apply(lines, edits), res, nil
}

func checkNames(add map[string]any) error {
	for name := range add {
		if _, ok := defaults[name]; !ok {
			return fmt.Errorf("unknown capability %q", name)
		}
	}
	return nil
}

// capabilitiesBlock returns the index of the block's key in root (-1 when missing) and its mapping, a new empty one
// when the key is missing. empty reports a key whose value is null or an empty mapping: its mapping is a new block
// style one, so a {} placeholder is not written as one long flow line.
func capabilitiesBlock(root *yaml.Node) (i int, block *yaml.Node, empty bool, err error) {
	i = keyIndex(root, blockKey)
	if i < 0 {
		return i, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, false, nil
	}
	value := root.Content[i+1]
	switch {
	case value.Kind == yaml.MappingNode && len(value.Content) == 0,
		value.Kind == yaml.ScalarNode && value.Tag == "!!null":
		return i, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, true, nil
	case value.Kind == yaml.MappingNode:
		return i, value, false, nil
	default:
		return i, nil, false, fmt.Errorf("%s in the manifest is not a mapping", blockKey)
	}
}

// keepCommentLines puts the comment and blank lines of an empty block's old range (the lines after its key) back
// under the key line of its new text. yaml.v3 attaches them to the document or to the next key, so the encoder
// would not write them.
func keepCommentLines(text string, old []string) string {
	var kept strings.Builder
	for _, line := range old {
		if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept.WriteString(strings.TrimRight(line, "\r\n") + "\n")
		}
	}
	keyLine, rest, _ := strings.Cut(text, "\n")
	return keyLine + "\n" + kept.String() + rest
}

// setCapability sets or skips one capability and reports whether the block changed.
func setCapability(block *yaml.Node, name string, value any, force bool, res *Result) (bool, error) {
	i := keyIndex(block, name)
	if i >= 0 && !force {
		res.Skipped = append(res.Skipped, name)
		return false, nil
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return false, fmt.Errorf("failed to encode %s: %w", name, err)
	}
	if i >= 0 {
		block.Content[i+1] = &node
		res.Replaced = append(res.Replaced, name)
		return true, nil
	}
	block.Content = append(block.Content, scalar(name), &node)
	res.Added = append(res.Added, name)
	return true, nil
}

// removeInstanceCapabilities returns the instance.capabilities scalar entries in file order and the splice that
// removes the key: its lines, or the whole re-encoded instance entry when instance is written in flow style.
func removeInstanceCapabilities(root *yaml.Node, lines []string, indent int) ([]string, *splice, error) {
	i := keyIndex(root, "instance")
	if i < 0 || root.Content[i+1].Kind != yaml.MappingNode {
		return nil, nil, nil
	}
	instance := root.Content[i+1]
	j := keyIndex(instance, "capabilities")
	if j < 0 {
		return nil, nil, nil
	}
	var removed []string
	for _, entry := range instance.Content[j+1].Content {
		if entry.Kind == yaml.ScalarNode {
			removed = append(removed, entry.Value)
		}
	}
	from, to := entryRange(root, i, len(lines), lines)
	if instance.Style&yaml.FlowStyle != 0 {
		instance.Content = append(instance.Content[:j], instance.Content[j+2:]...)
		text, err := encodeEntry(root.Content[i], instance, indent)
		if err != nil {
			return nil, nil, err
		}
		return removed, &splice{from, to, text}, nil
	}
	from, to = entryRange(instance, j, to, lines)
	return removed, &splice{from, to, ""}, nil
}

// entryRange returns the 0-based line range [from, to) of the key at index i of a block mapping: from the key's
// line to the next key's line, or to end for the last key. Blank lines and comments no deeper than the key that
// close the range stay outside it, since they lead into what follows.
func entryRange(mapping *yaml.Node, i, end int, lines []string) (int, int) {
	key := mapping.Content[i]
	from, to := key.Line-1, end
	if i+2 < len(mapping.Content) {
		to = mapping.Content[i+2].Line - 1
	}
	for to > from+1 {
		line := lines[to-1]
		trimmed := strings.TrimSpace(line)
		depth := len(line) - len(strings.TrimLeft(line, " \t"))
		if trimmed != "" && (!strings.HasPrefix(trimmed, "#") || depth > key.Column-1) {
			break
		}
		to--
	}
	return from, to
}

// detectIndent returns the column step from the first top-level key with a nested block mapping to that mapping's
// first key, or 2 when there is none.
func detectIndent(root *yaml.Node) int {
	for i := 0; i+1 < len(root.Content); i += 2 {
		value := root.Content[i+1]
		if value.Kind == yaml.MappingNode && value.Style&yaml.FlowStyle == 0 && len(value.Content) > 0 {
			if step := value.Content[0].Column - root.Content[i].Column; step > 0 {
				return step
			}
		}
	}
	return 2
}

// encodeEntry encodes one top-level key and its value. The key's head and foot comments are dropped, because they
// sit on lines outside the entry's range and stay in the file.
func encodeEntry(key, value *yaml.Node, indent int) (string, error) {
	k := *key
	k.HeadComment, k.FootComment = "", ""
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{&k, value}}); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// apply makes the non-overlapping splices on lines. Text added after a last line without a newline gets one first.
func apply(lines []string, edits []splice) []byte {
	slices.SortFunc(edits, func(a, b splice) int { return a.from - b.from })
	var out strings.Builder
	next := 0
	for _, e := range edits {
		for _, line := range lines[next:e.from] {
			out.WriteString(line)
		}
		if e.from == len(lines) && e.from > 0 && !strings.HasSuffix(lines[e.from-1], "\n") {
			out.WriteString("\n")
		}
		out.WriteString(e.text)
		next = e.to
	}
	for _, line := range lines[next:] {
		out.WriteString(line)
	}
	return []byte(out.String())
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

// WriteFileAtomic replaces an existing file with data through a temporary file in the same directory, keeping the
// file's mode. A symbolic link stays a link and its target is replaced. The temporary file never outlives a failure.
func WriteFileAtomic(path string, data []byte) (err error) {
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
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
