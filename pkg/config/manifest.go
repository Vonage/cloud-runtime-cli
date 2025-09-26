package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrNoManifest = errors.New("manifest file not found")
var ErrNotExistedPath = errors.New("path is not existed")
var ErrNotDirectory = errors.New("path is not a directory")

var (
	DefaultManifestFileNames = []string{
		"neru.yaml",
		"neru.yml",
		"vcr.yaml",
		"vcr.yml",
	}
)

type Manifest struct {
	Project  Project  `yaml:"project"`
	Instance Instance `yaml:"instance"`
	Debug    Debug    `yaml:"debug,omitempty"`
}

type Project struct {
	Name string `yaml:"name"`
}

type Env struct {
	Name   string `json:"name"   yaml:"name"`
	Value  string `json:"value"  yaml:"value,omitempty"`
	Secret string `json:"secret" yaml:"secret,omitempty"`
}

type Scaling struct {
	MinScale int `yaml:"min-scale,omitempty"`
	MaxScale int `yaml:"max-scale,omitempty"`
}

type Instance struct {
	Name          string            `yaml:"name"`
	Runtime       string            `yaml:"runtime,omitempty"`
	Region        string            `yaml:"region,omitempty"`
	ApplicationID string            `yaml:"application-id,omitempty"`
	Environment   []Env             `yaml:"environment,omitempty"`
	Capabilities  []string          `yaml:"capabilities,omitempty"`
	Entrypoint    []string          `yaml:"entrypoint,omitempty"`
	Domains       []string          `yaml:"domains,omitempty"`
	BuildScript   string            `yaml:"build-script,omitempty"`
	Scaling       Scaling           `yaml:"scaling,omitempty"`
	PathAccess    map[string]string `yaml:"path-access,omitempty"`
}

type Debug struct {
	Name          string   `yaml:"name,omitempty"`
	ApplicationID string   `yaml:"application-id,omitempty"`
	Environment   []Env    `yaml:"environment,omitempty"`
	Entrypoint    []string `yaml:"entrypoint,omitempty"`
	PreserveData  bool     `yaml:"preserve-data,omitempty"`
}

func NewManifestWithDefaults() *Manifest {
	return &Manifest{
		Instance: Instance{
			ApplicationID: "<please create an application on VONAGE APIs dashboard>",
			Entrypoint:    []string{"<please include the entrypoint for your script>"},
		},
		Debug: Debug{
			ApplicationID: "<please create an application on VONAGE APIs dashboard>",
			Entrypoint:    []string{"<please include the entrypoint for your script>"},
		},
	}
}

func GetAbsFilename(path string, filename string) string {
	return filepath.Join(path, filename)
}

func GetAbsDir(path string) (string, error) {
	var absPath string
	var err error

	if path == "" {
		path, err = os.Getwd()
		if err != nil {
			return "", err
		}
		return path, nil
	}

	absPath, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return absPath, ErrNotExistedPath
		}
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotDirectory
	}
	return absPath, nil
}

// ReadManifest reads the manifest from the given path.
func ReadManifest(path string) (*Manifest, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := yaml.Unmarshal(fileData, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func FindManifestFile(path, cwd string) (string, error) {
	var resultPath string
	var err error
	if path != "" {
		resultPath, err = FindFlagManifestFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to find flag manifest file: %w", err)
		}
		return resultPath, nil
	}

	resultPath, err = FindTemplateManifestFile(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to find template manifest file: %w", err)
	}

	return resultPath, nil
}

func FindFlagManifestFile(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return "", ErrNoManifest
}

func FindTemplateManifestFile(path string) (string, error) {
	var mostRecentModTime time.Time
	var mostRecentFile string

	for _, file := range DefaultManifestFileNames {
		fileInfo, err := os.Stat(GetAbsFilename(path, file))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		modTime := fileInfo.ModTime()
		if modTime.After(mostRecentModTime) {
			mostRecentModTime = modTime
			mostRecentFile = GetAbsFilename(path, file)
		}
	}
	if mostRecentFile == "" {
		return "", ErrNoManifest
	}
	return mostRecentFile, nil
}

// DefaultFilePermission defines the default permission for manifest files
const DefaultFilePermission = 0644

// WriteManifest writes the manifest to the given path.
func WriteManifest(path string, manifest *Manifest) error {
	file, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}

	err = os.WriteFile(path, file, DefaultFilePermission)
	if err != nil {
		return err
	}
	return nil
}

func Merge(original, override *Manifest) *Manifest {
	original.Project.Name = override.Project.Name
	original.Instance.Name = override.Instance.Name
	original.Instance.ApplicationID = override.Instance.ApplicationID
	original.Debug.ApplicationID = override.Debug.ApplicationID
	original.Instance.Region = override.Instance.Region
	original.Instance.Runtime = override.Instance.Runtime
	return original
}
