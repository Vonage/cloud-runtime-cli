// Package manifest finds, parses, merges and writes vcr.yml (and its older names).
package manifest

// Names are the manifest file names v2.6.1 looks for in a project directory; the newest modification time wins.
var Names = []string{"neru.yaml", "neru.yml", "vcr.yaml", "vcr.yml"}

// Manifest mirrors v2.6.1's pkg/config.Manifest plus application-capabilities, non-strict parse (unknown keys ignored).
type Manifest struct {
	Project  Project  `yaml:"project"`
	Instance Instance `yaml:"instance"`
	Debug    Debug    `yaml:"debug,omitempty"`
	// ApplicationCapabilities is the Vonage application "capabilities" object exactly as written in vcr.yml. The
	// CLI sends it as is; only CheckApplicationCapabilities looks inside.
	ApplicationCapabilities map[string]any `yaml:"application-capabilities,omitempty"`
}

type Project struct {
	Name string `yaml:"name"`
}

// Env is also sent as-is in deploy and validate requests, so its JSON tags are wire contract.
type Env struct {
	Name   string `json:"name"   yaml:"name"`
	Value  string `json:"value"  yaml:"value,omitempty"`
	Secret string `json:"secret" yaml:"secret,omitempty"`
}

type Scaling struct {
	MinScale int `yaml:"min-scale,omitempty"`
	MaxScale int `yaml:"max-scale,omitempty"`
}

// Security is sent as-is in deploy requests.
type Security struct {
	Access     string       `json:"access" yaml:"access"`
	AuthMethod string       `json:"authMethod,omitempty" yaml:"auth-method,omitempty"`
	Override   []PathAccess `json:"override,omitempty" yaml:"override,omitempty"`
}

type PathAccess struct {
	Path       string `json:"path" yaml:"path"`
	Access     string `json:"access" yaml:"access"`
	AuthMethod string `json:"authMethod,omitempty" yaml:"auth-method,omitempty"`
}

type Instance struct {
	Name            string    `yaml:"name"`
	Runtime         string    `yaml:"runtime,omitempty"`
	Region          string    `yaml:"region,omitempty"`
	ApplicationID   string    `yaml:"application-id,omitempty"`
	Environment     []Env     `yaml:"environment,omitempty"`
	Capabilities    []string  `yaml:"capabilities,omitempty"`
	Entrypoint      []string  `yaml:"entrypoint,omitempty"`
	Domains         []string  `yaml:"domains,omitempty"`
	BuildScript     string    `yaml:"build-script,omitempty"`
	Scaling         Scaling   `yaml:"scaling,omitempty"`
	Security        *Security `yaml:"security,omitempty"`
	HealthCheckPath string    `yaml:"health-check-path,omitempty"`
}

type Debug struct {
	Name          string   `yaml:"name,omitempty"`
	ApplicationID string   `yaml:"application-id,omitempty"`
	Environment   []Env    `yaml:"environment,omitempty"`
	Entrypoint    []string `yaml:"entrypoint,omitempty"`
	PreserveData  bool     `yaml:"preserve-data,omitempty"`
}
