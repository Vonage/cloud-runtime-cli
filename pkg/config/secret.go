package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/cli/cli/v2/pkg/iostreams"
)

var ErrNoSecretFile = errors.New("no secret file found")

type Secret struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func NewSecret(name, value string) Secret {
	return Secret{
		Name:  name,
		Value: value,
	}
}

func FindSecretFile(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return "", ErrNoSecretFile
}

var envarFormat = "^[a-zA-Z_]+[a-zA-Z0-9_]*$"

func ValidateSecretName(secretName string) (bool, error) {
	if !regexp.MustCompile(envarFormat).MatchString(secretName) {
		return false, fmt.Errorf("must follow the regex format %q", envarFormat)
	}

	return true, nil
}

func GetSecretFromInputs(out *iostreams.IOStreams, name, value, secretFile string) (Secret, error) {
	switch {
	case len(value) > 0:
		return NewSecret(name, value), nil
	case len(secretFile) > 0:
		_, err := FindSecretFile(secretFile)
		if err != nil {
			return Secret{}, err
		}

		fileData, err := os.ReadFile(secretFile)
		if err != nil {
			return Secret{}, err
		}
		if len(fileData) == 0 {
			return Secret{}, fmt.Errorf("no value provided for secret")
		}
		return NewSecret(name, string(fileData)), nil

	default:
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			fmt.Fprintf(out.Out, "Reading from STDIN - hit (Control + D) to stop.\n")
		}
		secretBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return Secret{}, err
		}
		if len(secretBytes) == 0 {
			return Secret{}, fmt.Errorf("no value provided for secret")
		}
		return NewSecret(name, string(secretBytes)), nil
	}
}
