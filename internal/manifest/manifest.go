// Package manifest finds, parses, merges and writes vcr.yml (and its older names).
package manifest

// Names are the manifest file names v2.6.1 looks for in a project directory; the newest modification time wins.
var Names = []string{"neru.yaml", "neru.yml", "vcr.yaml", "vcr.yml"}

// Manifest mirrors v2.6.1's pkg/config.Manifest: same YAML keys, non-strict parse (unknown keys ignored).
type Manifest struct {
	Project  Project  `yaml:"project"`
	Instance Instance `yaml:"instance"`
	Debug    Debug    `yaml:"debug,omitempty"`
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

// Find returns the manifest path for a project directory: flagPath as-is when set, otherwise the newest of Names
// in dir. It returns v2.6.1's errors when nothing is found.
func Find(dir, flagPath string) (string, error) { panic("not implemented: manifest.Find") }

// Read parses a manifest file.
func Read(path string) (*Manifest, error) { panic("not implemented: manifest.Read") }

// Parse is the non-strict YAML parse Read uses, for bytes read elsewhere (scaffold reads through an os.Root).
func Parse(data []byte) (*Manifest, error) { panic("not implemented: manifest.Parse") }

// ProjectDir turns the optional [path] argument of deploy, debug and init into an absolute directory with v2.6.1's
// checks and error texts. For a path that does not exist it returns the absolute path together with an error matching
// fs.ErrNotExist (init creates the directory; deploy and debug report the error); a path that is not a directory is
// an error.
func ProjectDir(path string) (string, error) { panic("not implemented: manifest.ProjectDir") }

// Merge returns a new manifest: base with exactly the six values v2.6.1's Merge copies from override, unconditionally
// (project name, instance name, instance application id, debug application id, region, runtime). Inputs are not
// modified.
func Merge(base, override *Manifest) *Manifest { panic("not implemented: manifest.Merge") }

// Marshal returns the YAML bytes v2.6.1 writes for m.
func Marshal(m *Manifest) ([]byte, error) { panic("not implemented: manifest.Marshal") }

// Capabilities are capability versions (e.g. Messages "v0.1", Voice "v0"). They are sent as-is in package and
// debug-service requests, so the JSON tags are wire contract.
type Capabilities struct {
	Messages string `json:"messages,omitempty"`
	Voice    string `json:"voice,omitempty"`
	RTC      string `json:"rtc,omitempty"`
	Video    string `json:"video,omitempty"`
	Verify   string `json:"verify,omitempty"`
	Network  string `json:"network,omitempty"`
}

// ParseCapabilities reproduces v2.6.1's format.ParseCapabilities, including its prefix matching and default versions
// ("messages"/"messaging" → v0.1; "voice", "rtc", "video", "network" → v0; "<name>-<version>" otherwise).
func ParseCapabilities(caps []string) (Capabilities, error) {
	panic("not implemented: manifest.ParseCapabilities")
}
