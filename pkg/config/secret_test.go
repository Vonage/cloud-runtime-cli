package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSecretName(t *testing.T) {
	secretName := "MY_SECRET"
	expectedResult := true
	expectedError := error(nil)

	result, err := ValidateSecretName(secretName)

	if result != expectedResult {
		t.Errorf("Expected result: %v, but got: %v", expectedResult, result)
	}

	if err != expectedError {
		t.Errorf("Expected error: %v, but got: %v", expectedError, err)
	}

	secretName = "my-secret"
	expectedResult = false
	expectedError = fmt.Errorf("must follow the regex format %q", envarFormat)

	result, err = ValidateSecretName(secretName)

	if result != expectedResult {
		t.Errorf("Expected result: %v, but got: %v", expectedResult, result)
	}

	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error: %v, but got: %v", expectedError, err)
	}
}

func TestGetSecretFromInputs(t *testing.T) {
	name := "my-secret"
	value := "my-value"
	secretFile := ""
	expectedSecret := Secret{
		Name:  name,
		Value: value,
	}
	expectedError := error(nil)

	secret, err := GetSecretFromInputs(nil, name, value, secretFile)

	require.Equal(t, expectedSecret, secret)
	require.Equal(t, expectedError, err)

	name = "my-secret"
	value = ""
	tempDir := t.TempDir()
	secretFile = filepath.Join(tempDir, "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("secretValue"), 0644))
	expectedSecret = Secret{
		Name:  name,
		Value: "secretValue",
	}
	expectedError = error(nil)

	secret, err = GetSecretFromInputs(nil, name, value, secretFile)

	require.Equal(t, expectedSecret, secret)
	require.Equal(t, expectedError, err)

	name = "my-secret"
	value = ""
	secretFile = "testdata/secret.txt"
	expectedError = ErrNoSecretFile

	_, err = GetSecretFromInputs(nil, name, value, secretFile)

	require.Equal(t, expectedError, err)

	name = "my-secret"
	value = ""
	secretFile = ""
	stdinData := "my-stdin-value"
	expectedSecret = Secret{
		Name:  name,
		Value: stdinData,
	}
	expectedError = error(nil)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte(stdinData))
	w.Close()

	secret, err = GetSecretFromInputs(nil, name, value, secretFile)

	require.Equal(t, expectedSecret, secret)
	require.Equal(t, expectedError, err)
}
