package manifest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/manifest"
)

// answers is what an override carries: the six values Merge takes, plus keys Merge must leave alone. Each call builds
// a new value.
func answers() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{Name: "other-app"},
		Instance: manifest.Instance{
			Name:          "prod",
			Runtime:       "python3",
			Region:        "aws.use1",
			ApplicationID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Environment:   []manifest.Env{{Name: "X", Value: "y"}},
			Entrypoint:    []string{"python", "main.py"},
		},
		Debug: manifest.Debug{Name: "other-debug", ApplicationID: "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee"},
	}
}

// scribble writes through every slice and pointer of m, which changes any other manifest that shares them.
func scribble(m *manifest.Manifest) {
	m.Instance.Environment[0].Name = "scribbled"
	m.Instance.Capabilities[0] = "scribbled"
	m.Instance.Entrypoint[0] = "scribbled"
	m.Instance.Domains[0] = "scribbled"
	m.Instance.Security.Access = "scribbled"
	m.Instance.Security.Override[0].Path = "scribbled"
	m.Debug.Environment[0].Name = "scribbled"
	m.Debug.Entrypoint[0] = "scribbled"
}

func TestMergeCopiesExactlySixValues(t *testing.T) {
	base := fullManifest()
	override := answers()

	got := manifest.Merge(base, override)

	want := fullManifest()
	want.Project.Name = "other-app"
	want.Instance.Name = "prod"
	want.Instance.Runtime = "python3"
	want.Instance.Region = "aws.use1"
	want.Instance.ApplicationID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	want.Debug.ApplicationID = "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee"
	assert.Equal(t, want, got)
	assert.Equal(t, fullManifest(), base, "base is not modified")
	assert.Equal(t, answers(), override, "override is not modified")
}

func TestMergeCopiesEmptyValues(t *testing.T) {
	override := answers()
	override.Instance.Region = ""

	got := manifest.Merge(fullManifest(), override)

	assert.Empty(t, got.Instance.Region)
	assert.Equal(t, "prod", got.Instance.Name)
}

func TestMergeSharesNothingWithTheBase(t *testing.T) {
	base := fullManifest()

	got := manifest.Merge(base, answers())
	scribble(got)
	got.Instance.Entrypoint = append(got.Instance.Entrypoint[:1], "appended")

	assert.Equal(t, fullManifest(), base)
}

func TestMergeKeepsAbsentValuesAbsent(t *testing.T) {
	got := manifest.Merge(&manifest.Manifest{}, &manifest.Manifest{})

	assert.Equal(t, &manifest.Manifest{}, got)
}
