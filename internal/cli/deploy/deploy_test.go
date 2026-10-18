package deploy

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/manifest"
)

func TestFlagDefinitions(t *testing.T) {
	cmd := NewCmd(&cmdenv.Env{})
	tests := []struct {
		name, shorthand, usage string
	}{
		{"project-name", "p", "Project name (overrides manifest value)"},
		{"runtime", "r", "Runtime environment, e.g., nodejs18, nodejs20, python3 (overrides manifest)"},
		{"app-id", "i", "Vonage application UUID to link with this deployment (overrides manifest)"},
		{"instance-name", "n", "Instance name, e.g., dev, staging, prod (overrides manifest)"},
		{"capabilities", "c", "Comma-separated capabilities: messages-v1,voice,rtc (overrides manifest)"},
		{"tgz", "z", "Path to pre-compressed tar.gz file to deploy (skips local compression)"},
		{"filename", "f", "Path to manifest file (default: vcr.yml in project directory)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.name)

			require.NotNil(t, flag)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
			assert.Equal(t, "string", flag.Value.Type())
			assert.Empty(t, flag.DefValue)
			assert.Equal(t, tt.usage, flag.Usage)
			assert.False(t, flag.Hidden)
		})
	}
	var declared []string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { declared = append(declared, f.Name) })
	assert.Len(t, declared, len(tests), "the command declares no other local flag")
}

func TestCommandShape(t *testing.T) {
	cmd := NewCmd(&cmdenv.Env{})

	assert.Equal(t, "deploy [path_to_code]", cmd.Use)
	assert.Equal(t, "Deploy your application to Vonage Cloud Runtime", cmd.Short)
	assert.Equal(t, cmdenv.ConfigOnly, cmd.Annotations[cmdenv.Annotation])
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.Nil(t, cmd.PersistentPreRunE, "the root owns the persistent hooks")
	assert.Nil(t, cmd.PersistentPreRun)
	assert.Nil(t, cmd.PersistentPostRun)
	assert.Nil(t, cmd.PersistentPostRunE)

	require.NoError(t, cmd.Args(cmd, nil))
	require.NoError(t, cmd.Args(cmd, []string{"./project"}))
	assert.EqualError(t, cmd.Args(cmd, []string{"a", "b"}), "accepts at most 1 arg(s), received 2")
}

func TestBuildPlanPrecedence(t *testing.T) {
	read := &manifest.Manifest{
		Project: manifest.Project{Name: "from-manifest"},
		Instance: manifest.Instance{
			Name: "manifest-instance", Runtime: "manifest-runtime", ApplicationID: "manifest-app",
		},
	}
	tests := []struct {
		name  string
		flags flagValues
		read  *manifest.Manifest
		want  [4]string // project name, instance name, runtime, application id
	}{
		{
			name:  "the flags beat the manifest",
			flags: flagValues{projectName: "p", instanceName: "i", runtime: "r", appID: "a"},
			read:  read,
			want:  [4]string{"p", "i", "r", "a"},
		},
		{
			name: "the manifest is used where a flag is empty",
			read: read,
			want: [4]string{"from-manifest", "manifest-instance", "manifest-runtime", "manifest-app"},
		},
		{
			name:  "one flag does not hide the other values",
			flags: flagValues{instanceName: "qa"},
			read:  read,
			want:  [4]string{"from-manifest", "qa", "manifest-runtime", "manifest-app"},
		},
		{
			name: "both empty gives empty, with no required check here",
			read: &manifest.Manifest{},
			want: [4]string{"", "", "", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := buildPlan(tt.flags, tt.read, "/project", "aws.euw1")

			assert.Equal(t, tt.want, [4]string{plan.ProjectName, plan.InstanceName, plan.Runtime, plan.AppID})
			assert.Equal(t, tt.want[0], plan.Manifest.Project.Name, "the merged manifest carries the same values")
			assert.Equal(t, tt.want[1], plan.Manifest.Instance.Name)
			assert.Equal(t, tt.want[2], plan.Manifest.Instance.Runtime)
			assert.Equal(t, tt.want[3], plan.Manifest.Instance.ApplicationID)
			assert.Equal(t, "/project", plan.ProjectDir)
			assert.Equal(t, "aws.euw1", plan.Region)
		})
	}
}

func TestBuildPlanLeavesTheManifestItReadUntouched(t *testing.T) {
	read := &manifest.Manifest{Project: manifest.Project{Name: "from-manifest"}}

	plan := buildPlan(flagValues{projectName: "other"}, read, "/project", "aws.euw1")

	assert.Equal(t, "from-manifest", read.Project.Name)
	assert.Equal(t, "other", plan.Manifest.Project.Name)
	assert.NotSame(t, read, plan.Manifest)
}

func TestBuildPlanPassesTheFlagsThatHaveNoManifestValue(t *testing.T) {
	plan := buildPlan(flagValues{tgz: "dist/app.tar.gz"}, &manifest.Manifest{}, "/project", "aws.euw1")

	assert.Equal(t, "dist/app.tar.gz", plan.TgzPath, "the archive path is used as typed")
}

func TestCapabilitiesFlagReplacesTheManifestList(t *testing.T) {
	read := &manifest.Manifest{Instance: manifest.Instance{Capabilities: []string{"rtc"}}}
	tests := []struct {
		name string
		flag string
		want []string
	}{
		{"the flag replaces the list", "messages-v1,voice", []string{"messages-v1", "voice"}},
		{"nothing is trimmed", "messages-v1, voice", []string{"messages-v1", " voice"}},
		{"an empty flag keeps the manifest list", "", []string{"rtc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := buildPlan(flagValues{capabilities: tt.flag}, read, "/project", "aws.euw1")

			assert.Equal(t, tt.want, plan.Capabilities)
			assert.Equal(t, tt.want, plan.Manifest.Instance.Capabilities)
		})
	}
}

func TestCapabilitiesStayNilWhenNothingNamesAny(t *testing.T) {
	plan := buildPlan(flagValues{}, &manifest.Manifest{}, "/project", "aws.euw1")

	assert.Nil(t, plan.Capabilities)
}

func TestEffectiveRegion(t *testing.T) {
	withRegion := func(region string) *manifest.Manifest {
		return &manifest.Manifest{Instance: manifest.Instance{Region: region}}
	}
	tests := []struct {
		name       string
		flag       string
		read       *manifest.Manifest
		config     string
		wantRegion string
		wantSource string
	}{
		{"the flag beats the manifest and the config", "aws.flag", withRegion("aws.manifest"), "aws.config", "aws.flag", fromFlag},
		{"the manifest beats the config", "", withRegion("aws.manifest"), "aws.config", "aws.manifest", fromManifest},
		{"the config is the last resort", "", withRegion(""), "aws.config", "aws.config", fromConfig},
		{"the flag alone", "aws.flag", withRegion(""), "", "aws.flag", fromFlag},
		{"the manifest alone", "", withRegion("aws.manifest"), "", "aws.manifest", fromManifest},
		{"nothing anywhere", "", withRegion(""), "", "", fromNowhere},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, source := effectiveRegion(tt.flag, tt.read, tt.config)

			assert.Equal(t, tt.wantRegion, region)
			assert.Equal(t, tt.wantSource, source)
		})
	}
}
