package initcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// dirty is remote text with a colour sequence, a title-setting sequence and a bell around the word red.
const dirty = "\x1b[31mred\x1b[0m\x1b]0;t\a"

func TestRuntimeOptions(t *testing.T) {
	runtimes := []platform.Runtime{
		{Name: "nodejs18", Language: "nodejs"},
		{Name: "nodejs22", Language: "nodejs", Comments: "latest"},
		{Name: "debug-runtime", Language: "debug"},
		{Name: "nodejs14", Language: "nodejs", Comments: "deprecated"},
		{Name: "no-language"},
		{Name: "python3", Language: "python"},
	}

	got := runtimeOptions(runtimes)

	assert.Equal(t, []string{"nodejs18", "nodejs22 - (latest)", "python3"}, got.labels)
	assert.Equal(t, runtimeChoice{name: "nodejs18", language: "nodejs"}, got.values["nodejs18"])
	assert.Equal(t, runtimeChoice{name: "nodejs22", language: "nodejs"}, got.values["nodejs22 - (latest)"])
	assert.Equal(t, runtimeChoice{name: "python3", language: "python"}, got.values["python3"])
	assert.Len(t, got.values, 3)
}

func TestRuntimeDefault(t *testing.T) {
	tests := []struct {
		name     string
		runtimes []platform.Runtime
		want     string
	}{
		{"the plain label is the default", []platform.Runtime{{Name: "nodejs18", Language: "nodejs"}}, "nodejs18"},
		{"a commented label is not", []platform.Runtime{{Name: "nodejs18", Language: "nodejs", Comments: "lts"}}, ""},
		{"another runtime only", []platform.Runtime{{Name: "nodejs22", Language: "nodejs"}}, ""},
		{"none", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, runtimeDefault(runtimeOptions(tt.runtimes)))
		})
	}
}

func TestRegionOptions(t *testing.T) {
	regions := []platform.Region{
		{Name: "AWS Europe (Ireland)", Alias: "aws.euw1"},
		{Name: "Test region", Alias: "aws.test1"},
		{Name: "Prod", Alias: "aws.mytest"},
		{Name: "AWS US East", Alias: "aws.use1"},
	}

	got := newRegionOptions(regions)

	assert.Equal(t, []string{"AWS Europe (Ireland) - (aws.euw1)", "AWS US East - (aws.use1)"}, got.labels)
	assert.Equal(t, "aws.euw1", got.values["AWS Europe (Ireland) - (aws.euw1)"])
	assert.Equal(t, "aws.use1", got.values["AWS US East - (aws.use1)"])
}

func TestRegionDefault(t *testing.T) {
	got := newRegionOptions([]platform.Region{
		{Name: "AWS Europe (Ireland)", Alias: "aws.euw1"},
		{Name: "Test region", Alias: "aws.test1"},
	})

	assert.Equal(t, "AWS Europe (Ireland) - (aws.euw1)", got.defaultLabel("aws.euw1"))
	assert.Empty(t, got.defaultLabel("aws.use1"), "a region the platform does not list")
	assert.Empty(t, got.defaultLabel("aws.test1"), "a region that is not offered")
	assert.Empty(t, got.defaultLabel(""))
}

func TestAppOptions(t *testing.T) {
	got := appOptions([]platform.Application{{ID: "a1", Name: "App One"}, {ID: "a2", Name: "App Two"}})

	assert.Equal(t, []string{"SKIP", "CREATE NEW APP", "App One - (a1)", "App Two - (a2)"}, got.labels)
	assert.Equal(t, "a1", got.values["App One - (a1)"])
	assert.Equal(t, "a2", got.values["App Two - (a2)"])
	assert.Len(t, got.values, 2, "SKIP and CREATE NEW APP stand for no application")
}

func TestTheLaterEntryWinsWhenLabelsRepeat(t *testing.T) {
	templates := templateOptions([]platform.Template{{ID: "t1", Name: "Same"}, {ID: "t2", Name: "Same"}})
	assert.Equal(t, []string{"SKIP", "Same", "Same"}, templates.labels)
	assert.Equal(t, "t2", templates.values["Same"].ID)

	regions := newRegionOptions([]platform.Region{{Name: "One", Alias: "aws.x"}, {Name: "Two", Alias: "aws.x"}})
	assert.Equal(t, "Two - (aws.x)", regions.defaultLabel("aws.x"))
}

func TestTemplatesForPutsStartersFirst(t *testing.T) {
	templates := []platform.Template{
		{ID: "t1", Name: "Voice App", ProgrammingLanguage: "nodejs"},
		{ID: "t2", Name: "starter project - Express", ProgrammingLanguage: "NodeJS"},
		{ID: "t3", Name: "Python Thing", ProgrammingLanguage: "python"},
		{ID: "t4", Name: "Starter Project - Node.js", ProgrammingLanguage: "nodejs"},
		{ID: "t5", Name: "Messaging", ProgrammingLanguage: "nodejs"},
	}

	got := templatesFor("nodejs", templates)

	var ids []string
	for _, template := range got {
		ids = append(ids, template.ID)
	}
	assert.Equal(t, []string{"t2", "t4", "t1", "t5"}, ids)
	assert.Empty(t, templatesFor("go", templates))
}

func TestTemplateOptions(t *testing.T) {
	templates := []platform.Template{
		{ID: "t1", Name: "Starter Project - Node.js", ProgrammingLanguage: "nodejs", ActiveVersionID: "v1"},
		{ID: "t2", Name: "Voice App", ProgrammingLanguage: "nodejs", ActiveVersionID: "v2"},
	}

	got := templateOptions(templates)

	assert.Equal(t, []string{"SKIP", "Starter Project - Node.js", "Voice App"}, got.labels)
	assert.Equal(t, templates[0], got.values["Starter Project - Node.js"])
	assert.Equal(t, templates[1], got.values["Voice App"])
	assert.Len(t, got.values, 2)
}

func TestLabelsAreSanitised(t *testing.T) {
	t.Run("runtime", func(t *testing.T) {
		got := runtimeOptions([]platform.Runtime{
			{Name: dirty, Language: "nodejs"},
			{Name: "x", Language: "nodejs", Comments: dirty},
		})

		assert.Equal(t, []string{"red", "x - (red)"}, got.labels)
		assert.Equal(t, dirty, got.values["red"].name, "the value is what the platform sent")
	})
	t.Run("region", func(t *testing.T) {
		got := newRegionOptions([]platform.Region{{Name: dirty, Alias: dirty}})

		assert.Equal(t, []string{"red - (red)"}, got.labels)
		assert.Equal(t, dirty, got.values["red - (red)"])
		assert.Equal(t, "red - (red)", got.defaultLabel(dirty))
	})
	t.Run("application", func(t *testing.T) {
		got := appOptions([]platform.Application{{Name: dirty, ID: dirty}})

		assert.Equal(t, []string{"SKIP", "CREATE NEW APP", "red - (red)"}, got.labels)
		assert.Equal(t, dirty, got.values["red - (red)"])
	})
	t.Run("template", func(t *testing.T) {
		got := templateOptions([]platform.Template{{ID: "t1", Name: dirty}})

		assert.Equal(t, []string{"SKIP", "red"}, got.labels)
		assert.Equal(t, dirty, got.values["red"].Name)
	})
}

func TestProjectNamePattern(t *testing.T) {
	for _, name := range []string{"a", "a1", "my-project", "a-b-c", "0", "9-lives"} {
		assert.True(t, validProjectName(name), "%q", name)
	}
	for _, name := range []string{"", "-a", "a-", "A", "a_b", "a b", "a.b", "ab\n", "-"} {
		assert.False(t, validProjectName(name), "%q", name)
	}
}
