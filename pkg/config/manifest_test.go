package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadManifest(t *testing.T) {
	existingPath := "testdata/vcr.yaml"
	manifest, err := ReadManifest(existingPath)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	expectedManifest := &Manifest{
		Project: Project{
			Name: "test",
		},
		Instance: Instance{
			Name: "dev",
		},
		Debug: Debug{
			ApplicationID: "id",
		},
	}
	if !reflect.DeepEqual(manifest, expectedManifest) {
		t.Errorf("Expected manifest to be %+v, but got %+v", expectedManifest, manifest)
	}

	nonExistingPath := "testdata/nonexisting.yaml"
	_, err = ReadManifest(nonExistingPath)
	if err == nil {
		t.Errorf("Expected an error, but got nil")
	}

}

func TestGetAbsDir(t *testing.T) {
	emptyPath, err := GetAbsDir("")
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	currentDir, _ := os.Getwd()
	if emptyPath != currentDir {
		t.Errorf("Expected empty path to be %s, but got %s", currentDir, emptyPath)
	}

	existingDir := "testdata"
	absPath, err := GetAbsDir(existingDir)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	expectedAbsPath, _ := filepath.Abs(existingDir)
	if absPath != expectedAbsPath {
		t.Errorf("Expected absPath to be %s, but got %s", expectedAbsPath, absPath)
	}

	nonExistingDir := "nonexisting"
	_, err = GetAbsDir(nonExistingDir)
	if err != ErrNotExistedPath {
		t.Errorf("Expected ErrNotExistedPath, but got: %v", err)
	}

	filePath := "manifest.go"
	_, err = GetAbsDir(filePath)
	if err != ErrNotDirectory {
		t.Errorf("Expected ErrNotDirectory, but got: %v", err)
	}
}

func TestFindManifestFile(t *testing.T) {
	path := "testdata/vcr.yaml"
	cwd := ""
	resultPath, err := FindManifestFile(path, cwd)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	if resultPath != path {
		t.Errorf("Expected resultPath to be %s, but got %s", path, resultPath)
	}

	path = ""
	cwd = "testdata"
	resultPath, err = FindManifestFile(path, cwd)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	expectedPath := "testdata/vcr.yaml"
	if resultPath != expectedPath {
		t.Errorf("Expected resultPath to be %s, but got %s", expectedPath, resultPath)
	}

	path = ""
	cwd = "nonexisting"
	_, err = FindManifestFile(path, cwd)
	expectedErr := fmt.Errorf("failed to find template manifest file: %w", ErrNoManifest)
	if err.Error() != expectedErr.Error() {
		t.Errorf("Expected error: %v, but got: %v", expectedErr, err)
	}
}

func TestWriteManifest(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	tmpFile := filepath.Join(tempDir, "fixture.txt")

	// Define a sample manifest
	manifest := &Manifest{Project: Project{Name: "test"}}

	// Write the manifest to the temporary file
	err := WriteManifest(tmpFile, manifest)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	// Read the content of the temporary file
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read temporary file: %v", err)
	}

	// Verify the content of the written manifest
	expectedContent := "project:\n    name: test\ninstance:\n    name: \"\"\n"
	testContent := string(content)
	if testContent != expectedContent {
		t.Errorf("Expected content to be:\n%s\nBut got:\n%s", expectedContent, string(content))
	}
}

func TestMerge(t *testing.T) {
	original := &Manifest{
		Project: Project{
			Name: "Original Project",
		},
		Instance: Instance{
			Name:          "Original Instance",
			ApplicationID: "original-app-id",
			Region:        "original-region",
			Runtime:       "original-runtime",
		},
		Debug: Debug{
			ApplicationID: "original-debug-app-id",
		},
	}

	override := &Manifest{
		Project: Project{
			Name: "Override Project",
		},
		Instance: Instance{
			Name:          "Override Instance",
			ApplicationID: "override-app-id",
			Region:        "override-region",
			Runtime:       "override-runtime",
		},
		Debug: Debug{
			ApplicationID: "override-debug-app-id",
		},
	}

	result := Merge(original, override)

	expected := &Manifest{
		Project: Project{
			Name: "Override Project",
		},
		Instance: Instance{
			Name:          "Override Instance",
			ApplicationID: "override-app-id",
			Region:        "override-region",
			Runtime:       "override-runtime",
		},
		Debug: Debug{
			ApplicationID: "override-debug-app-id",
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected merged manifest to be %+v, but got %+v", expected, result)
	}
}
