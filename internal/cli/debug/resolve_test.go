package debug

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fallbackAppIDWarning = "! A debug application-id was not detected in the manifest, the instance application-id " +
	"was loaded as an alternative. Please consider adding a debug application-id\n"

func TestApplicationIDPrecedenceAndWarning(t *testing.T) {
	const flagID = "00000000-0000-4000-8000-0000000000ff"
	instanceOnly := `project: {name: p}
instance: {name: dev, runtime: nodejs18, region: aws.euw1, application-id: ` + instanceAppID + `}
`
	neither := `project: {name: p}
instance: {name: dev, runtime: nodejs18, region: aws.euw1}
`
	tests := []struct {
		name       string
		manifest   string
		args       []string
		want       string
		wantStdout string
		wantErr    string
	}{
		{name: "the flag wins", manifest: fullManifest, args: []string{"--app-id", flagID}, want: flagID},
		{name: "the short flag wins too", manifest: fullManifest, args: []string{"-i", flagID}, want: flagID},
		{name: "the debug id of the manifest", manifest: fullManifest, want: debugAppID},
		{name: "the instance id says so", manifest: instanceOnly, want: instanceAppID, wantStdout: fallbackAppIDWarning},
		{name: "the flag needs no warning", manifest: instanceOnly, args: []string{"--app-id", flagID}, want: flagID},
		{name: "no id at all", manifest: neither, wantErr: "failed to get debug app id: application-id is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)

			err := f.run(append(tc.args, f.project(tc.manifest))...)

			assert.Equal(t, tc.wantStdout, f.stdout.String())
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				assert.Zero(t, f.rec.count(), "no session without an application")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, f.rec.only(t).cfg.AppID)
		})
	}
}

func TestRuntimeRequired(t *testing.T) {
	withRuntime := fullManifest
	withoutRuntime := `project: {name: p}
instance: {name: dev, region: aws.euw1}
debug: {application-id: ` + debugAppID + `, entrypoint: [node]}
`
	tests := []struct {
		name     string
		manifest string
		args     []string
		want     string
		wantErr  string
	}{
		{name: "the runtime of the instance", manifest: withRuntime, want: "nodejs18"},
		{name: "the flag wins", manifest: withRuntime, args: []string{"--runtime", "python3"}, want: "python3"},
		{name: "the short flag", manifest: withRuntime, args: []string{"-r", "nodejs22"}, want: "nodejs22"},
		{name: "the flag fills a gap", manifest: withoutRuntime, args: []string{"-r", "python3"}, want: "python3"},
		{name: "no runtime", manifest: withoutRuntime, wantErr: "failed to get runtime: runtime is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)

			err := f.run(append(tc.args, f.project(tc.manifest))...)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				assert.Zero(t, f.rec.count())
				assert.Empty(t, f.stdout.String(), "a missing runtime has no fallback and prints no warning")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, f.rec.only(t).cfg.Runtime)
			assert.Empty(t, f.stdout.String(), "the runtime never prints a warning")
		})
	}
}

func TestNameAndPreserveData(t *testing.T) {
	named := `project: {name: p}
instance: {name: dev, runtime: nodejs18, region: aws.euw1}
debug: {application-id: ` + debugAppID + `, name: from-manifest, preserve-data: true, entrypoint: [node]}
`
	tests := []struct {
		name         string
		manifest     string
		args         []string
		wantName     string
		wantPreserve bool
	}{
		{name: "nothing is set", manifest: fullManifest},
		{name: "the name of the manifest", manifest: named, wantName: "from-manifest", wantPreserve: true},
		{name: "the flag names the proxy", manifest: named, args: []string{"--name", "mine"}, wantName: "mine", wantPreserve: true},
		{name: "the short flag", manifest: fullManifest, args: []string{"-n", "short"}, wantName: "short"},
		{name: "the flag turns data preservation on", manifest: fullManifest, args: []string{"--preserve-data"}, wantPreserve: true},
		{name: "the flag cannot switch it off", manifest: named, args: []string{"--preserve-data=false"}, wantName: "from-manifest", wantPreserve: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)

			require.NoError(t, f.run(append(tc.args, f.project(tc.manifest))...))

			cfg := f.rec.only(t).cfg
			assert.Equal(t, tc.wantName, cfg.Name)
			assert.Equal(t, tc.wantPreserve, cfg.PreserveData)
		})
	}
}

func TestProjectDirectoryAndFilename(t *testing.T) {
	t.Run("the argument is the project directory", func(t *testing.T) {
		f := newFixture(t, false)
		dir := f.project(fullManifest)

		require.NoError(t, f.run(dir))

		cfg := f.rec.only(t).cfg
		assert.Equal(t, dir, cfg.ProjectDir)
		assert.Equal(t, []string{"node", "index.js"}, cfg.Manifest.Debug.Entrypoint)
		assert.Equal(t, "my-project", cfg.Manifest.Project.Name)
	})

	t.Run("no argument means the working directory", func(t *testing.T) {
		f := newFixture(t, false)
		dir := f.project(fullManifest)
		t.Chdir(dir)

		require.NoError(t, f.run())

		want, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, want, f.rec.only(t).cfg.ProjectDir)
	})

	t.Run("the filename names the manifest", func(t *testing.T) {
		f := newFixture(t, false)
		dir := writeProject(t, "custom.yml", fullManifest)

		require.NoError(t, f.run("--filename", filepath.Join(dir, "custom.yml"), dir))

		assert.Equal(t, "my-project", f.rec.only(t).cfg.Manifest.Project.Name)
	})

	t.Run("the short flag of the filename", func(t *testing.T) {
		f := newFixture(t, false)
		dir := writeProject(t, "custom.yml", fullManifest)

		require.NoError(t, f.run("-f", filepath.Join(dir, "custom.yml"), dir))

		assert.Equal(t, 1, f.rec.count())
	})

	t.Run("a filename that does not exist", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.run("--filename", filepath.Join(t.TempDir(), "missing.yml"), f.project(fullManifest))

		require.EqualError(t, err, "failed to find flag manifest file: manifest file not found")
		assert.Zero(t, f.rec.count())
	})

	t.Run("a directory without a manifest", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.run(t.TempDir())

		require.EqualError(t, err, "failed to find template manifest file: manifest file not found")
		assert.Zero(t, f.rec.count())
	})
}

func TestProjectDirectoryErrors(t *testing.T) {
	t.Run("a path that does not exist", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.run("nope")

		require.EqualError(t, err, `failed to get absolute path of "nope": path is not existed`)
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.Zero(t, f.rec.count())
	})

	t.Run("a path that is a file", func(t *testing.T) {
		f := newFixture(t, false)
		file := filepath.Join(f.project(fullManifest), "vcr.yml")

		err := f.run(file)

		require.EqualError(t, err, `failed to get absolute path of "`+file+`": path is not a directory`)
		assert.Zero(t, f.rec.count())
	})

	t.Run("nothing is asked of the platform before the directory is known", func(t *testing.T) {
		f := newFixture(t, false)

		require.Error(t, f.run("nope"))

		assert.Empty(t, f.srv.Requests())
	})
}

func TestManifestReadError(t *testing.T) {
	f := newFixture(t, false)
	dir := f.project("project: [unclosed\n")

	err := f.run(dir)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read manifest file: ")
	assert.Zero(t, f.rec.count())
	assert.Empty(t, f.srv.Requests(), "a manifest that cannot be read is reported before the region is looked up")
}

func TestTooManyArguments(t *testing.T) {
	f := newFixture(t, false)

	err := f.run("one", "two")

	require.Error(t, err)
	assert.Zero(t, f.rec.count())
}
