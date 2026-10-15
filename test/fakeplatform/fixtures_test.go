package fakeplatform

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultsAreSelfConsistent(t *testing.T) {
	f := Defaults()
	shape := regexp.MustCompile(`^00000000-0000-4000-8000-[0-9]{12}$`)
	owner := map[string]string{}
	claim := func(what, kind, id string) {
		t.Helper()
		assert.Regexp(t, shape, id, what)
		assert.True(t, strings.HasPrefix(id[len(id)-4:], kind), "%s: id %s is not of kind %s", what, id, kind)
		if prev, dup := owner[id]; dup {
			t.Errorf("id %s is used by %s and %s", id, prev, what)
		}
		owner[id] = what
	}

	for _, r := range f.Runtimes {
		claim("runtime "+r.Name, "01", r.ID)
	}
	for _, p := range f.Products {
		claim("product "+p.Name, "02", p.ID)
		claim("active version of "+p.Name, "03", p.ActiveVersionID)
	}
	projects := map[string]bool{}
	for _, p := range f.Projects {
		claim("project "+p.Name, "04", p.ID)
		projects[p.Name] = true
	}
	apps := map[string]bool{}
	for _, a := range f.Applications {
		claim("application "+a.Name, "06", a.ID)
		apps[a.ID] = true
	}
	instances := map[string]bool{}
	for _, in := range f.Instances {
		claim("instance "+in.Name, "05", in.ID)
		instances[in.ID] = true
		assert.True(t, projects[in.ProjectName], "instance %s: project %q does not exist", in.Name, in.ProjectName)
		assert.True(t, apps[in.APIApplicationID], "instance %s: application %q does not exist", in.Name, in.APIApplicationID)
	}
	for i, l := range f.Logs {
		assert.True(t, instances[l.Instance], "log %d: instance %q does not exist", i, l.Instance)
	}
	claim("deployed project", "07", f.Deploy.ProjectID)
	claim("deployed package", "07", f.Deploy.PackageID)
	claim("deployed instance", "07", f.Deploy.InstanceID)
	claim("deployment", "07", f.Deploy.DeploymentID)

	assert.NoError(t, f.validate())
}

func TestDefaultsDescribeTheDocumentedWorld(t *testing.T) {
	f := Defaults()
	assert.Equal(t, Account{Key: testKey, Secret: testSecret}, f.Account)

	require.Len(t, f.Regions, 2)
	assert.Equal(t, []string{"aws.euw1", "aws.use1"}, []string{f.Regions[0].Alias, f.Regions[1].Alias})
	assert.True(t, f.Regions[0].Default)
	assert.False(t, f.Regions[1].Default)
	assert.Equal(t, "${FAKE}/v1/vcr/use1/api/deployment", f.Regions[1].DeploymentAPIURL)
	assert.Equal(t, "http://${OTHER_HOST}", f.Regions[1].HostTemplate)

	names := make([]string, len(f.Instances))
	for i, in := range f.Instances {
		names[i] = in.Name
	}
	assert.Equal(t, []string{"dev", "staging", "prod", "gone"}, names)
	assert.True(t, f.Instances[3].Deleted)
	assert.Equal(t, "aws.use1", f.Instances[2].Region)

	assert.Equal(t, []string{"DB_URL", "API_TOKEN"}, f.Secrets)
	require.Len(t, f.Logs, 5)
	assert.Equal(t, "provider call failed", f.Logs[3].Message)
	assert.Equal(t, "2026-10-14T10:00:05Z", f.Logs[4].Timestamp)
	assert.Equal(t, "source-key-0001", f.Deploy.SourceCodeKey)
	assert.True(t, f.Deploy.Validate.Valid)
	assert.Empty(t, f.Overrides)
}

func TestDecodeOverlaysDefaults(t *testing.T) {
	f, err := Decode([]byte("secrets: [A]\naccount: {key: other-key}\ndeploy: {validate: {valid: false}}\n"))
	require.NoError(t, err)
	d := Defaults()

	assert.Equal(t, []string{"A"}, f.Secrets, "a list replaces the default list")
	assert.Equal(t, d.Regions, f.Regions)
	assert.Equal(t, d.Instances, f.Instances)
	assert.Equal(t, "other-key", f.Account.Key)
	assert.Equal(t, d.Account.Secret, f.Account.Secret, "a mapping merges into the default")
	assert.False(t, f.Deploy.Validate.Valid)
	assert.Equal(t, d.Deploy.PackageID, f.Deploy.PackageID)

	empty, err := Decode(nil)
	require.NoError(t, err)
	assert.Equal(t, d, empty)

	cleared, err := Decode([]byte("secrets: []\n"))
	require.NoError(t, err)
	assert.Empty(t, cleared.Secrets)
}

func TestDecodeRejectsUnknownKeys(t *testing.T) {
	for _, y := range []string{
		"nope: 1\n",
		"regions: [{alias: aws.x, colour: red}]\n",
		"deploy: {validate: {valid: true, why: x}}\n",
		"overrides: [{match: GET /secrets, statuss: 500}]\n",
	} {
		_, err := Decode([]byte(y))
		assert.Error(t, err, y)
	}
}

func TestDecodeRejectsInvalidValues(t *testing.T) {
	for name, y := range map[string]string{
		"empty account key":       "account: {key: ''}\n",
		"log timestamp":           "logs: [{instance: x, timestamp: yesterday}]\n",
		"instance updatedAt":      "instances: [{id: x, updatedAt: soon}]\n",
		"match without a route":   "overrides: [{match: GET}]\n",
		"unknown match kind":      "overrides: [{match: 'GET ftp:/x'}]\n",
		"double star not last":    "overrides: [{match: 'GET /a/**/b'}]\n",
		"negative skip":           "overrides: [{match: 'GET /a', skip: -1}]\n",
		"bad delay":               "overrides: [{match: 'GET /a', delay: soon}]\n",
		"logs on a text body":     "overrides: [{match: 'GET /a', status: 500, body: oops, containerLogs: x}]\n",
		"status out of range":     "overrides: [{match: 'GET /a', status: 1000}]\n",
		"hang and close together": "overrides: [{match: 'GET /a', hang: true, close: true}]\n",
	} {
		_, err := Decode([]byte(y))
		assert.Error(t, err, name)
	}
}
