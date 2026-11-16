// Command schemagen writes schema/vcr.schema.json, the JSON Schema for vcr.yml, from the Vonage Application API v2
// definition. Run it from the repository root with "make schema SPEC=<file>"; it is a developer tool, not a vcr
// command.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const refPrefix = "#/components/schemas/"

// dropped are the OpenAPI-only keys (besides x-*) that draft-07 does not know or that only describe API responses.
var dropped = map[string]bool{
	"example": true, "nullable": true, "readOnly": true, "writeOnly": true, "deprecated": true,
	"discriminator": true, "xml": true, "externalDocs": true,
}

func main() {
	spec := flag.String("spec", "", "the Vonage Application API v2 definition (OpenAPI YAML)")
	out := flag.String("out", "schema/vcr.schema.json", "the schema file to write")
	flag.Parse()
	if *spec == "" {
		fmt.Fprintln(os.Stderr, "schemagen: -spec is required")
		os.Exit(2)
	}
	if err := run(*spec, *out); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run(specPath, outPath string) error {
	spec, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	caps, err := capabilities(spec)
	if err != nil {
		return err
	}
	data, err := encode(manifestSchema(caps))
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644) //nolint:gosec // a committed, public file
}

// capabilities returns components.schemas.ApplicationResponse.properties.capabilities as a draft-07 schema.
func capabilities(spec []byte) (map[string]any, error) {
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	caps, ok := lookup(doc.Components.Schemas, "ApplicationResponse", "properties", "capabilities").(map[string]any)
	if !ok {
		return nil, errors.New("components.schemas.ApplicationResponse.properties.capabilities not found")
	}
	c := converter{schemas: doc.Components.Schemas, resolving: map[string]bool{}}
	s, err := c.schema(caps, "capabilities")
	if err != nil {
		return nil, err
	}
	return s.(map[string]any), nil //nolint:forcetypeassert // schema returns a map for a map
}

func lookup(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

type converter struct {
	schemas   map[string]any
	resolving map[string]bool // the refs being inlined, to stop on a cycle
}

// schema converts the OpenAPI schema object v, found at path, to draft-07.
func (c converter) schema(v any, path string) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: a schema must be a mapping with string keys, got %T", path, v)
	}
	if ref, ok := m["$ref"]; ok {
		return c.inline(ref, path)
	}
	out := map[string]any{}
	for k, val := range m {
		if dropped[k] || strings.HasPrefix(k, "x-") {
			continue
		}
		conv, err := c.keyword(k, val, path)
		if err != nil {
			return nil, err
		}
		out[k] = conv
	}
	if m["nullable"] == true {
		if t, ok := out["type"].(string); ok {
			out["type"] = []any{t, "null"}
		}
	}
	if d, ok := out["default"].(string); ok && out["type"] == "boolean" && (d == "true" || d == "false") {
		out["default"] = d == "true"
	}
	if out["type"] == "object" || out["properties"] != nil {
		out["additionalProperties"] = true
	}
	return out, nil
}

func (c converter) inline(ref any, path string) (any, error) {
	s, _ := ref.(string)
	name, local := strings.CutPrefix(s, refPrefix)
	target, found := c.schemas[name]
	if !local || !found {
		return nil, fmt.Errorf("%s: unsupported $ref %q: only local %s refs can be inlined", path, ref, refPrefix)
	}
	if c.resolving[name] {
		return nil, fmt.Errorf("%s: $ref %q refers to itself", path, s)
	}
	c.resolving[name] = true
	defer delete(c.resolving, name)
	return c.schema(target, path)
}

// keyword converts the value of keyword k: subschemas are converted, everything else is copied.
func (c converter) keyword(k string, v any, path string) (any, error) {
	switch k {
	case "properties":
		props, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.properties: a mapping is required, got %T", path, v)
		}
		out := make(map[string]any, len(props))
		for name, p := range props {
			conv, err := c.schema(p, path+"."+name)
			if err != nil {
				return nil, err
			}
			out[name] = conv
		}
		return out, nil
	case "items", "not":
		return c.schema(v, path+"."+k)
	case "allOf", "anyOf", "oneOf":
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s: a list is required, got %T", path, k, v)
		}
		out := make([]any, len(list))
		for i, s := range list {
			conv, err := c.schema(s, fmt.Sprintf("%s.%s[%d]", path, k, i))
			if err != nil {
				return nil, err
			}
			out[i] = conv
		}
		return out, nil
	default:
		return v, nil
	}
}

// encode writes v as sorted, two-space-indented JSON with a trailing newline, so a regeneration is a no-op.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
