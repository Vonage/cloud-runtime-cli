package deploy

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"

	"vonage-cloud-runtime-cli/internal/platform"
)

// Names of the calls fakeAPI records, in the order Run is expected to make them.
const (
	callGetProject      = "GetProject"
	callCreateProject   = "CreateProject"
	callValidate        = "ValidateDeployment"
	callUploadSource    = "UploadSource"
	callCreatePackage   = "CreatePackage"
	callWatchBuild      = "WatchBuild"
	callDeployInstance  = "DeployInstance"
	maxUploadedBodySize = 1 << 20
)

// fakeAPI is a hand-written API that records the order of the calls and the requests it received, and answers from
// its fields.
type fakeAPI struct {
	calls []string

	// answers
	project       platform.Project
	getProjectErr error
	newProjectID  string
	createErr     error
	validation    platform.ValidateResult
	validateErr   error
	sourceKey     string
	uploadErr     error
	packageID     string
	packageErr    error
	frames        []string
	watchErr      error
	deployed      platform.DeployResult
	deployErr     error

	// hooks run first in the call they are named after, to let a test end the context in the middle of a run
	onValidate func()
	// onCall runs first in every call with the name of the method, to let a test move the clock inside a call
	onCall func(name string)

	// what the calls received
	validateReq  platform.ValidateRequest
	packageReq   platform.PackageRequest
	deployReq    platform.DeployRequest
	projectAsked string
	createdName  string
	uploadedPath string
	entries      []string // tar entry names of the archive that was uploaded
	watchedID    string
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		project:    platform.Project{ID: "p-1", Name: "my-project"},
		validation: platform.ValidateResult{Valid: true},
		sourceKey:  "source-key-1",
		packageID:  "pkg-1",
		frames:     []string{"building", "Status - completed"},
		deployed: platform.DeployResult{
			InstanceID:  "i-1",
			ServiceName: "my-project-dev",
			HostURLs:    []string{"https://one.example.test", "https://two.example.test"},
		},
	}
}

// enter records a call and runs the hook before the method does anything else.
func (f *fakeAPI) enter(name string) {
	f.calls = append(f.calls, name)
	if f.onCall != nil {
		f.onCall(name)
	}
}

func (f *fakeAPI) called(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

func (f *fakeAPI) GetProject(_ context.Context, name string) (platform.Project, error) {
	f.enter(callGetProject)
	f.projectAsked = name
	if f.getProjectErr != nil {
		return platform.Project{}, f.getProjectErr
	}
	return f.project, nil
}

func (f *fakeAPI) CreateProject(_ context.Context, name string) (string, error) {
	f.enter(callCreateProject)
	f.createdName = name
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.newProjectID, nil
}

func (f *fakeAPI) ValidateDeployment(_ context.Context, req platform.ValidateRequest) (platform.ValidateResult, error) {
	f.enter(callValidate)
	f.validateReq = req
	if f.onValidate != nil {
		f.onValidate()
	}
	if f.validateErr != nil {
		return platform.ValidateResult{}, f.validateErr
	}
	return f.validation, nil
}

// UploadSource reads the archive while it is still there: the file is removed when Run returns.
func (f *fakeAPI) UploadSource(_ context.Context, path string) (string, error) {
	f.enter(callUploadSource)
	f.uploadedPath = path
	f.entries = tarEntries(path)
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	return f.sourceKey, nil
}

func (f *fakeAPI) CreatePackage(_ context.Context, req platform.PackageRequest) (string, error) {
	f.enter(callCreatePackage)
	f.packageReq = req
	if f.packageErr != nil {
		return "", f.packageErr
	}
	return f.packageID, nil
}

func (f *fakeAPI) WatchBuild(_ context.Context, packageID string, onLine func(frame string)) error {
	f.enter(callWatchBuild)
	f.watchedID = packageID
	for _, frame := range f.frames {
		onLine(frame)
	}
	return f.watchErr
}

func (f *fakeAPI) DeployInstance(_ context.Context, req platform.DeployRequest) (platform.DeployResult, error) {
	f.enter(callDeployInstance)
	f.deployReq = req
	if f.deployErr != nil {
		return platform.DeployResult{}, f.deployErr
	}
	return f.deployed, nil
}

// tarEntries returns the names in the tar.gz at path, or nil when it is not one.
func tarEntries(path string) []string {
	raw, err := os.Open(path) //nolint:gosec // the path is the archive Run passed to the upload
	if err != nil {
		return nil
	}
	defer raw.Close()
	zr, err := gzip.NewReader(raw)
	if err != nil {
		return nil
	}
	defer zr.Close()
	var names []string
	tr := tar.NewReader(io.LimitReader(zr, maxUploadedBodySize))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return names
		}
		if err != nil {
			return nil
		}
		names = append(names, hdr.Name)
	}
}
