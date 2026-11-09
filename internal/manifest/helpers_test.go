package manifest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const (
	goldenFull     = "full.yml"
	goldenDefaults = "defaults.yml"
)

// fullManifest is the value testdata/full.yml spells out, with every key set. Each call builds a new value, so a test
// can keep an untouched copy next to the one it hands to the code under test.
func fullManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{Name: "booking-app"},
		Instance: manifest.Instance{
			Name:          "dev",
			Runtime:       "nodejs22",
			Region:        "aws.euw1",
			ApplicationID: "11111111-2222-3333-4444-555555555555",
			Environment: []manifest.Env{
				{Name: "VONAGE_NUMBER", Value: "12012010601"},
				{Name: "API_KEY", Secret: "MY_SECRET"},
			},
			Capabilities: []string{"messages-v1", "voice"},
			Entrypoint:   []string{"node", "index.js"},
			Domains:      []string{"api.example.com"},
			BuildScript:  "./build.sh",
			Scaling:      manifest.Scaling{MinScale: 1, MaxScale: 3},
			Security: &manifest.Security{
				Access:     "private",
				AuthMethod: "vonage_basic",
				Override: []manifest.PathAccess{
					{Path: "/api/public", Access: "public"},
					{Path: "/api/secure", Access: "authenticated", AuthMethod: "vonage_basic"},
				},
			},
			HealthCheckPath: "/custom/health",
		},
		Debug: manifest.Debug{
			Name:          "debug",
			ApplicationID: "22222222-2222-3333-4444-555555555555",
			Environment:   []manifest.Env{{Name: "A", Value: "b"}},
			Entrypoint:    []string{"node", "--inspect", "index.js"},
			PreserveData:  true,
		},
	}
}

// defaultsManifest is the value testdata/defaults.yml spells out.
func defaultsManifest() *manifest.Manifest {
	const (
		applicationID = "<please create an application on VONAGE APIs dashboard>"
		entrypoint    = "<please include the entrypoint for your script>"
	)
	return &manifest.Manifest{
		Project: manifest.Project{Name: "my-project"},
		Instance: manifest.Instance{
			Name:          "dev",
			Runtime:       "nodejs18",
			Region:        "aws.euw1",
			ApplicationID: applicationID,
			Entrypoint:    []string{entrypoint},
		},
		Debug: manifest.Debug{
			ApplicationID: applicationID,
			Entrypoint:    []string{entrypoint},
		},
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	return readText(t, filepath.Join("testdata", name))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// writeFile writes a file and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func setModTime(t *testing.T, path string, when time.Time) {
	t.Helper()
	require.NoError(t, os.Chtimes(path, when, when))
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes, symbolic links and error texts are not exercised on Windows")
	}
}

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not bind the superuser")
	}
}

// symlinkOrSkip creates a symbolic link, skipping the test where links cannot be created.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
}
