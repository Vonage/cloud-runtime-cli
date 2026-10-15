package config

import (
	"errors"
	"fmt"
)

// defaultPaths are the files tried, in this order, when the given one does not exist.
var defaultPaths = []string{DefaultFlagValue, "~/.neru-cli"}

// Load applies v2.6.1's lookup: the given path; when it does not exist, ~/.vcr-cli then ~/.neru-cli. It returns
// the warning lines v2.6.1 prints on stdout (without the icon; the caller prints "<warning icon> <line>"):
//
//	Config file not found at "<path>", using "<found>"
//	Config file not found at "<path>", please use 'vcr configure' to create one. Trying to use flags...
//
// When fl.ConfigFileSet and the file does not exist, it fails with
// `config file "<path>" not found; create it with 'vcr configure --config-file <path>'`, unless both
// --api-key and --api-secret are set; then v2.6.1's fallback chain and its notices run unchanged.
func Load(fl Flags) (s Settings, warnings []string, err error) {
	path := fl.ConfigFile
	if path == "" {
		path = DefaultFlagValue
	}
	file, found, warning, err := locate(fl, path)
	if err != nil {
		return Settings{}, nil, err
	}
	if warning != "" {
		warnings = []string{warning}
	}
	return Settings{
		File:      file,
		FilePath:  found,
		APIKey:    firstNonEmpty(fl.APIKey, file.APIKey),
		APISecret: firstNonEmpty(fl.APISecret, file.APISecret),
		Region:    firstNonEmpty(fl.Region, file.DefaultRegion),
	}, warnings, nil
}

// locate reads the given file and, when it does not exist, the default ones. found is the path that was read,
// as written, and warning is the notice for a fallback.
func locate(fl Flags, path string) (file File, found, warning string, err error) {
	file, err = Read(path)
	if err == nil {
		return file, path, "", nil
	}
	if !errors.Is(err, ErrNotFound) {
		return File{}, "", "", err
	}
	if fl.ConfigFileSet && (fl.APIKey == "" || fl.APISecret == "") {
		return File{}, "", "", fmt.Errorf("config file %q not found; create it with 'vcr configure --config-file %s'", path, path)
	}
	for _, candidate := range defaultPaths {
		file, err = Read(candidate)
		if err == nil {
			return file, candidate, fmt.Sprintf("Config file not found at %q, using %q", path, candidate), nil
		}
		if !errors.Is(err, ErrNotFound) {
			return File{}, "", "", readFailure(candidate, err)
		}
	}
	return File{}, "", fmt.Sprintf("Config file not found at %q, please use 'vcr configure' to create one. Trying to use flags...", path), nil
}

func firstNonEmpty(preferred, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}
