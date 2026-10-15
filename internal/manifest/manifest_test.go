package manifest_test

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

func TestNamesOrder(t *testing.T) {
	assert.Equal(t, []string{"neru.yaml", "neru.yml", "vcr.yaml", "vcr.yml"}, manifest.Names)
}

func TestReadParsesEverySection(t *testing.T) {
	got, err := manifest.Read(filepath.Join("testdata", goldenFull))

	require.NoError(t, err)
	assert.Equal(t, fullManifest(), got)
}

func TestParseReadsTheSameBytesAsRead(t *testing.T) {
	got, err := manifest.Parse([]byte(readGolden(t, goldenFull)))

	require.NoError(t, err)
	assert.Equal(t, fullManifest(), got)
}

func TestReadIgnoresUnknownKeys(t *testing.T) {
	const misspeltKey = "enviroment" //nolint:misspell // the key is misspelt on purpose
	path := writeFile(t, t.TempDir(), "vcr.yml", "unknown: 1\ninstance:\n  "+misspeltKey+": z\n")

	got, err := manifest.Read(path)

	require.NoError(t, err)
	assert.Equal(t, &manifest.Manifest{}, got)
}

func TestReadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"no bytes":          "",
		"only a comment":    "# nothing here\n",
		"only a doc marker": "---\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := manifest.Read(writeFile(t, dir, "vcr.yml", content))

			require.NoError(t, err)
			assert.Equal(t, &manifest.Manifest{}, got)
		})
	}
}

func TestParseEmptyInput(t *testing.T) {
	got, err := manifest.Parse(nil)

	require.NoError(t, err)
	assert.Equal(t, &manifest.Manifest{}, got)
}

func TestReadErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("a missing file is the raw file error", func(t *testing.T) {
		missing := filepath.Join(dir, "missing.yml")

		got, err := manifest.Read(missing)

		require.ErrorIs(t, err, fs.ErrNotExist)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Same(t, pathErr, err, "the error is returned as the OS gave it")
		assert.Equal(t, missing, pathErr.Path)
		assert.Nil(t, got)
	})
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "broken syntax", content: "project: [", want: "did not find expected node content"},
		{name: "a value of the wrong type", content: "instance:\n  scaling:\n    min-scale: abc\n", want: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.Read(writeFile(t, dir, "vcr.yml", tt.content))

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), "failed to read manifest file")
			assert.Nil(t, got)
		})
	}
}

func TestMarshalGolden(t *testing.T) {
	tests := []struct {
		name   string
		m      *manifest.Manifest
		golden string
	}{
		{name: "what init writes without a template", m: defaultsManifest(), golden: goldenDefaults},
		{name: "every key set", m: fullManifest(), golden: goldenFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.Marshal(tt.m)

			require.NoError(t, err)
			assert.Equal(t, readGolden(t, tt.golden), string(got))
		})
	}
}

func TestMarshalEmptyManifest(t *testing.T) {
	got, err := manifest.Marshal(&manifest.Manifest{})

	require.NoError(t, err)
	assert.Equal(t, "project:\n    name: \"\"\ninstance:\n    name: \"\"\n", string(got))
}

func TestMarshalReadRoundTrip(t *testing.T) {
	data, err := manifest.Marshal(fullManifest())
	require.NoError(t, err)
	path := writeFile(t, t.TempDir(), "vcr.yml", string(data))

	got, err := manifest.Read(path)

	require.NoError(t, err)
	assert.Equal(t, fullManifest(), got)
}

func TestMarshalDropsKeysTheStructDoesNotModel(t *testing.T) {
	read, err := manifest.Parse([]byte("project:\n  name: p\n  owner: me\ninstance:\n  name: dev\n  extra: 1\nother: 2\n"))
	require.NoError(t, err)

	got, err := manifest.Marshal(read)

	require.NoError(t, err)
	assert.Equal(t, "project:\n    name: p\ninstance:\n    name: dev\n", string(got))
}

func TestWireJSON(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "env keeps an empty secret",
			value: manifest.Env{Name: "A", Value: "b"},
			want:  `{"name":"A","value":"b","secret":""}`,
		},
		{
			name: "security with an override",
			value: manifest.Security{
				Access:     "private",
				AuthMethod: "vonage_basic",
				Override:   []manifest.PathAccess{{Path: "/p", Access: "public"}},
			},
			want: `{"access":"private","authMethod":"vonage_basic","override":[{"path":"/p","access":"public"}]}`,
		},
		{
			name:  "security leaves out what is empty",
			value: manifest.Security{Access: "public"},
			want:  `{"access":"public"}`,
		},
		{
			name:  "no capabilities",
			value: manifest.Capabilities{},
			want:  `{}`,
		},
		{
			name:  "capabilities that are set",
			value: manifest.Capabilities{Messages: "v1", RTC: "v0"},
			want:  `{"messages":"v1","rtc":"v0"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.value)

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
