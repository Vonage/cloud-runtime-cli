package cmdutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringVar(t *testing.T) {
	str, err := StringVar("name", "value", "manifestValue", "configValue", false)
	require.NoError(t, err)
	require.Equal(t, "value", str)

	str, err = StringVar("name", "", "manifestValue", "configValue", false)
	require.NoError(t, err)
	require.Equal(t, "manifestValue", str)

	str, err = StringVar("name", "", "", "configValue", false)
	require.NoError(t, err)
	require.Equal(t, "configValue", str)

	str, err = StringVar("name", "", "", "", true)
	require.Error(t, err)
	require.Equal(t, "", str)
	require.EqualError(t, err, "name is required")
}

func TestAskYesNo(t *testing.T) {
	content := []byte("y")
	testFile, err := os.CreateTemp(t.TempDir(), "example")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testFile.Write(content); err != nil {
		t.Fatal(err)
	}

	if _, err := testFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	testSurvey := &Survey{}
	os.Stdin = testFile
	result := testSurvey.AskYesNo("do you want to continue?")

	require.Equal(t, true, result)

	if err := testFile.Close(); err != nil {
		t.Fatal(err)
	}
	os.Remove(testFile.Name())

	content = []byte("n")
	testFile, err = os.CreateTemp(t.TempDir(), "example")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testFile.Name())
	if _, err := testFile.Write(content); err != nil {
		t.Fatal(err)
	}

	if _, err := testFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	oldStdin = os.Stdin
	defer func() { os.Stdin = oldStdin }()

	testSurvey = &Survey{}
	os.Stdin = testFile
	result = testSurvey.AskYesNo("do you want to continue?")

	require.Equal(t, false, result)

	if err := testFile.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDisplaySpinnerMessageWithHandle(t *testing.T) {
	expectedMessage := "Loading..."
	s := DisplaySpinnerMessageWithHandle(expectedMessage)
	require.Equal(t, expectedMessage, s.Suffix)
	s.Stop()

	require.False(t, s.Active())
}

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name         string
		instanceID   string
		instanceName string
		projectName  string
		wantErr      bool
	}{
		{
			name:         "Test with empty parameters",
			instanceID:   "",
			instanceName: "",
			projectName:  "",
			wantErr:      true,
		},
		{
			name:         "Test with only instanceID",
			instanceID:   "123",
			instanceName: "",
			projectName:  "",
			wantErr:      false,
		},
		{
			name:         "Test with instanceName and projectName",
			instanceID:   "",
			instanceName: "testInstance",
			projectName:  "testProject",
			wantErr:      false,
		},
		{
			name:         "Test with only instanceName",
			instanceID:   "",
			instanceName: "testInstance",
			projectName:  "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateFlags(tt.instanceID, tt.instanceName, tt.projectName); (err != nil) != tt.wantErr {
				t.Errorf("ValidateFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
