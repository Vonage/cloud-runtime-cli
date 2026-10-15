package fakeplatform

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Fixtures is the world the fake platform serves. Its strings may hold the
// placeholders that Server.Expand replaces; Start expands them before serving.
type Fixtures struct {
	Account      Account       `yaml:"account"`
	Regions      []Region      `yaml:"regions"`
	Runtimes     []Runtime     `yaml:"runtimes"`
	Products     []Product     `yaml:"products"`
	Projects     []Project     `yaml:"projects"`
	Instances    []Instance    `yaml:"instances"`
	Logs         []LogLine     `yaml:"logs"`
	LogsFailure  bool          `yaml:"logsFailure"`
	Applications []Application `yaml:"applications"`
	Secrets      []string      `yaml:"secrets"`
	Deploy       Deploy        `yaml:"deploy"`
	Overrides    []Override    `yaml:"overrides"`
}

// Account is the one API account the fake accepts.
type Account struct {
	Key    string `yaml:"key"`
	Secret string `yaml:"secret"`
}

// Region is a region record. A region whose Enabled is unset is enabled.
type Region struct {
	Alias             string `yaml:"alias"`
	Name              string `yaml:"name"`
	Default           bool   `yaml:"default"`
	Enabled           *bool  `yaml:"enabled,omitempty"`
	DeploymentAPIURL  string `yaml:"deploymentApiUrl"`
	AssetsAPIURL      string `yaml:"assetsApiUrl"`
	MarketplaceAPIURL string `yaml:"marketplaceApiUrl"`
	EndpointURLScheme string `yaml:"endpointUrlScheme"`
	DebuggerURLScheme string `yaml:"debuggerUrlScheme"`
	HostTemplate      string `yaml:"hostTemplate"`
}

// Runtime is a runtime record.
type Runtime struct {
	ID         string `yaml:"id"`
	Name       string `yaml:"name"`
	Language   string `yaml:"language"`
	Comments   string `yaml:"comments"`
	APIVersion string `yaml:"apiVersion"`
}

// Product is a marketplace product. A product whose Public is unset is
// public; one that is not public, or is deleted, is not listed as a template.
type Product struct {
	ID                  string `yaml:"id"`
	Name                string `yaml:"name"`
	ProgrammingLanguage string `yaml:"programmingLanguage"`
	ActiveVersionID     string `yaml:"activeVersionId"`
	Public              *bool  `yaml:"public,omitempty"`
	Deleted             bool   `yaml:"deleted"`
}

// Project is a project of Account, which is the fixture account when empty.
type Project struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Account   string `yaml:"account"`
	CreatedAt string `yaml:"createdAt"`
	UpdatedAt string `yaml:"updatedAt"`
}

// Instance is a deployed instance. ProjectAccount owns its project (the
// fixture account when empty) and PackageOwner owns the package it runs; the
// caller sees an instance that is not deleted when it is either of them.
type Instance struct {
	ID               string `yaml:"id"`
	Name             string `yaml:"name"`
	ServiceName      string `yaml:"serviceName"`
	APIApplicationID string `yaml:"apiApplicationId"`
	ProjectName      string `yaml:"projectName"`
	Region           string `yaml:"region"`
	UpdatedAt        string `yaml:"updatedAt"`
	Deleted          bool   `yaml:"deleted"`
	ProjectAccount   string `yaml:"projectAccount"`
	PackageOwner     string `yaml:"packageOwner"`
}

// LogLine is a log row of the instance whose id is Instance. Polls of an
// instance's logs are counted, and the row is returned from poll number
// VisibleAfterPolls+1 on.
type LogLine struct {
	Instance          string `yaml:"instance"`
	Level             string `yaml:"level"`
	Source            string `yaml:"source"`
	Message           string `yaml:"message"`
	Timestamp         string `yaml:"timestamp"`
	VisibleAfterPolls int    `yaml:"visibleAfterPolls"`
}

// Application is an application of the account.
type Application struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// Deploy holds what the deploy routes answer.
type Deploy struct {
	ProjectID     string     `yaml:"projectId"`
	PackageID     string     `yaml:"packageId"`
	SourceCodeKey string     `yaml:"sourceCodeKey"`
	InstanceID    string     `yaml:"instanceId"`
	ServiceName   string     `yaml:"serviceName"`
	DeploymentID  string     `yaml:"deploymentId"`
	HostURLs      []string   `yaml:"hostUrls"`
	Validate      Validation `yaml:"validate"`
}

// Validation is the answer to a deployment validation.
type Validation struct {
	Valid  bool              `yaml:"valid"`
	Errors []ValidationError `yaml:"errors"`
}

// ValidationError is one problem that a validation reports.
type ValidationError struct {
	Field   string `yaml:"field"`
	Message string `yaml:"message"`
}

// fixtureID returns the fixture id 00000000-0000-4000-8000-00000000NNNN; the
// first two digits of NNNN name the kind of record.
func fixtureID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

func boolPtr(v bool) *bool { return &v }

// Defaults returns the default world: one account, two regions, two runtimes,
// a template product, two projects, four instances (one deleted), five log
// lines, two applications, two secrets and the answers of a deploy.
func Defaults() Fixtures {
	return Fixtures{
		Account: Account{Key: "abc123", Secret: "s3cret-for-tests"},
		Regions: []Region{
			defaultRegion("aws.euw1", "AWS - Europe (Ireland)", true),
			defaultRegion("aws.use1", "AWS - US East (N. Virginia)", false),
		},
		Runtimes: []Runtime{
			{ID: fixtureID(101), Name: "nodejs18", Language: "nodejs", APIVersion: "v0.3"},
			{ID: fixtureID(102), Name: "python3", Language: "python", APIVersion: "v0.3"},
		},
		Products: []Product{{
			ID: fixtureID(201), Name: "Starter Project - Node.js", ProgrammingLanguage: "nodejs",
			ActiveVersionID: fixtureID(301), Public: boolPtr(true),
		}},
		Projects: []Project{
			{ID: fixtureID(401), Name: "my-project", CreatedAt: "2026-10-01T10:00:00Z", UpdatedAt: "2026-10-14T10:00:00Z"},
			{ID: fixtureID(402), Name: "other", CreatedAt: "2026-10-02T10:00:00Z", UpdatedAt: "2026-10-12T10:00:00Z"},
		},
		Instances:    defaultInstances(),
		Logs:         defaultLogs(),
		Applications: []Application{{ID: fixtureID(601), Name: "my-app"}, {ID: fixtureID(602), Name: "second-app"}},
		Secrets:      []string{"DB_URL", "API_TOKEN"},
		Deploy: Deploy{
			ProjectID: fixtureID(701), PackageID: fixtureID(702), SourceCodeKey: "source-key-0001",
			InstanceID: fixtureID(703), ServiceName: "my-project-dev", DeploymentID: fixtureID(704),
			HostURLs: []string{"https://my-project-dev.euw1.runtime.example.test"},
			Validate: Validation{Valid: true},
		},
	}
}

func defaultRegion(alias, name string, isDefault bool) Region {
	base := "${FAKE}/v1/vcr/" + shortAlias(alias) + "/api/"
	return Region{
		Alias: alias, Name: name, Default: isDefault, Enabled: boolPtr(true),
		DeploymentAPIURL: base + "deployment", AssetsAPIURL: base + "assets", MarketplaceAPIURL: base + "marketplace",
		EndpointURLScheme: "http", DebuggerURLScheme: "ws", HostTemplate: "http://${OTHER_HOST}",
	}
}

func defaultInstances() []Instance {
	const project = "my-project"
	return []Instance{
		{
			ID: fixtureID(501), Name: "dev", ServiceName: "my-project-dev", APIApplicationID: fixtureID(601),
			ProjectName: project, Region: "aws.euw1", UpdatedAt: "2026-10-14T10:00:00Z",
		},
		{
			ID: fixtureID(502), Name: "staging", ServiceName: "my-project-staging", APIApplicationID: fixtureID(602),
			ProjectName: project, Region: "aws.euw1", UpdatedAt: "2026-10-13T10:00:00Z",
		},
		{
			ID: fixtureID(503), Name: "prod", ServiceName: "other-prod", APIApplicationID: fixtureID(601),
			ProjectName: "other", Region: "aws.use1", UpdatedAt: "2026-10-12T10:00:00Z",
		},
		{
			ID: fixtureID(504), Name: "gone", ServiceName: "my-project-gone", APIApplicationID: fixtureID(601),
			ProjectName: project, Region: "aws.euw1", UpdatedAt: "2026-10-11T10:00:00Z", Deleted: true,
		},
	}
}

func defaultLogs() []LogLine {
	const app, provider = "application", "provider"
	rows := []struct{ level, source, message string }{
		{"info", app, "starting"},
		{"info", app, "listening on 3000"},
		{"warn", app, "slow request"},
		{"error", provider, "provider call failed"},
		{"info", app, "shutting down"},
	}
	out := make([]LogLine, len(rows))
	for i, r := range rows {
		out[i] = LogLine{
			Instance: fixtureID(501), Level: r.level, Source: r.source, Message: r.message,
			Timestamp: fmt.Sprintf("2026-10-14T10:00:%02dZ", i+1),
		}
	}
	return out
}

// shortAlias is a region alias without its provider prefix: aws.euw1 → euw1.
func shortAlias(alias string) string {
	if _, short, ok := strings.Cut(alias, "."); ok {
		return short
	}
	return alias
}

// Decode reads fixtures from YAML over Defaults: a list given in y replaces
// the default list and a mapping merges into the default value. Unknown keys
// and invalid values are errors.
func Decode(y []byte) (Fixtures, error) {
	f := Defaults()
	dec := yaml.NewDecoder(bytes.NewReader(y))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return Fixtures{}, fmt.Errorf("decode fixtures: %w", err)
	}
	if err := f.validate(); err != nil {
		return Fixtures{}, err
	}
	return f, nil
}

func parseTime(v string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, v)
}

func (f *Fixtures) validate() error {
	var errs []error
	if f.Account.Key == "" || f.Account.Secret == "" {
		errs = append(errs, errors.New("account needs a key and a secret"))
	}
	for i, r := range f.Regions {
		if r.Alias == "" {
			errs = append(errs, fmt.Errorf("regions[%d] has no alias", i))
		}
	}
	for i, in := range f.Instances {
		if _, err := parseTime(in.UpdatedAt); in.UpdatedAt != "" && err != nil {
			errs = append(errs, fmt.Errorf("instances[%d].updatedAt: %w", i, err))
		}
	}
	for i, l := range f.Logs {
		if _, err := parseTime(l.Timestamp); err != nil {
			errs = append(errs, fmt.Errorf("logs[%d].timestamp: %w", i, err))
		}
	}
	for i := range f.Overrides {
		if err := f.Overrides[i].validate(); err != nil {
			errs = append(errs, fmt.Errorf("overrides[%d]: %w", i, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid fixtures: %w", errors.Join(errs...))
	}
	return nil
}
