package manifest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const schemaFile = "../../schema/vcr.schema.json"

var schemaCorpora = []string{"testdata", "../../test/contract/testdata/corpora"}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(schemaFile)
	require.NoError(t, err)
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("vcr.schema.json", doc))
	sch, err := c.Compile("vcr.schema.json")
	require.NoError(t, err)
	return sch
}

// validate checks a value decoded from YAML, after the JSON round trip an editor's YAML language server also makes.
func validate(t *testing.T, sch *jsonschema.Schema, v any) error {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	require.NoError(t, err)
	return sch.Validate(inst)
}

func validateYAML(t *testing.T, sch *jsonschema.Schema, src string) error {
	t.Helper()
	var v any
	require.NoError(t, yaml.Unmarshal([]byte(src), &v))
	return validate(t, sch, v)
}

// failedAt lists the instance locations of the innermost schema errors, as "/a/b" paths.
func failedAt(err error) []string {
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return nil
	}
	var paths []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			paths = append(paths, "/"+strings.Join(e.InstanceLocation, "/"))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(verr)
	return paths
}

func TestSchemaAcceptsEveryDefaultCapability(t *testing.T) {
	sch := compileSchema(t)
	block := map[string]any{}
	for _, name := range manifest.CapabilityNames {
		c, err := manifest.DefaultCapability(name, manifest.NetworkAPIs{
			ApplicationID: "00000000-0000-4000-8000-000000000001", RedirectURI: "{{instance_url}}/callback",
		})
		require.NoError(t, err)
		block[name] = c
	}
	doc := map[string]any{
		"project":                  map[string]any{"name": "booking-app"},
		"instance":                 map[string]any{"name": "dev"},
		"application-capabilities": block,
	}
	require.NoError(t, validate(t, sch, doc))
}

// corpusManifests returns the YAML files under the corpora that the CLI parses and that are manifests (a mapping
// with project or instance), and logs the ones it leaves out.
func corpusManifests(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range schemaCorpora {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if ext := filepath.Ext(path); ext != ".yml" && ext != ".yaml" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if reason := notAManifest(data); reason != "" {
				t.Logf("skipped %s: %s", path, reason)
				return nil
			}
			files = append(files, path)
			return nil
		})
		require.NoError(t, err)
	}
	return files
}

// notAManifest says why data is left out of the corpus check, or returns "" when it is a manifest the CLI accepts.
func notAManifest(data []byte) string {
	if _, err := manifest.Parse(data); err != nil {
		return "the CLI rejects it: " + err.Error()
	}
	var top map[string]any
	if yaml.Unmarshal(data, &top) != nil || (top["project"] == nil && top["instance"] == nil) {
		return "not a manifest"
	}
	return ""
}

func TestSchemaAcceptsEveryCorpusManifest(t *testing.T) {
	sch := compileSchema(t)
	files := corpusManifests(t)
	require.Contains(t, files, filepath.Join("testdata", "commented.yml"))
	require.True(t, slices.ContainsFunc(files, func(f string) bool { return strings.HasPrefix(f, "../../test/") }),
		"no contract corpus manifest found")
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, validateYAML(t, sch, string(data)))
		})
	}
}

func TestSchemaRejectsAWrongValueAtItsPath(t *testing.T) {
	sch := compileSchema(t)
	tests := []struct {
		name, src, path string
	}{
		{
			name: "a string min-scale",
			src:  "project: {name: p}\ninstance:\n  name: dev\n  scaling:\n    min-scale: \"two\"\n",
			path: "/instance/scaling/min-scale",
		},
		{
			name: "an unknown voice region",
			src:  "project: {name: p}\ninstance: {name: dev}\napplication-capabilities:\n  voice:\n    region: mars-1\n",
			path: "/application-capabilities/voice/region",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateYAML(t, sch, tt.src)
			require.Error(t, err)
			require.Equal(t, []string{tt.path}, failedAt(err))
		})
	}
}

// A key with no value, such as environment: with every entry commented out, decodes to null.
func TestSchemaAcceptsEmptyKeys(t *testing.T) {
	sch := compileSchema(t)
	src := "project:\n  name:\ninstance:\n  name:\n  environment:\n  entrypoint:\n  scaling:\n    min-scale:\ndebug:\n"
	_, err := manifest.Parse([]byte(src))
	require.NoError(t, err)
	require.NoError(t, validateYAML(t, sch, src))
}

func TestSchemaAcceptsAnEmptyApplicationCapabilitiesKey(t *testing.T) {
	sch := compileSchema(t)
	src := "project:\n  name: p\napplication-capabilities:\n"
	_, err := manifest.Parse([]byte(src))
	require.NoError(t, err)
	require.NoError(t, validateYAML(t, sch, src))
}

func TestSchemaAllowsAKeyItDoesNotDescribe(t *testing.T) {
	sch := compileSchema(t)
	src := "project: {name: p}\ninstance: {name: dev}\napplication-capabilities:\n  voice:\n    new_field: 1\n"
	require.NoError(t, validateYAML(t, sch, src))
}
