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
	testFile, err := os.CreateTemp("", "example")
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
	testFile, err = os.CreateTemp("", "example")
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
