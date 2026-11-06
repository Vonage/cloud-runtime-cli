package deploy

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"sync"

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
	callBuildStatus     = "BuildStatus"
	callDeployInstance  = "DeployInstance"
	maxUploadedBodySize = 1 << 20
)

// fakeAPI is a hand-written API that records the order of the calls and the requests it received, and answers from
// its fields. The build stage calls WatchBuild and BuildStatus from two goroutines, so mu guards what those touch.
type fakeAPI struct {
	mu    sync.Mutex
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
	watchBlocks   bool          // WatchBuild sends its frames, then holds until its context ends
	lateFrames    []string      // WatchBuild sends these once BuildStatus has answered a final status
	finalStatus   chan struct{} // closed when BuildStatus first answers a final status
	closeFinal    sync.Once
	watchCtxErr   error          // the error of WatchBuild's context when it returned
	statuses      []statusAnswer // BuildStatus answers these in order, the last repeating
	statusCalls   int
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
	// the error of WatchBuild's context when DeployInstance was called: non-nil means the stream was closed first
	deployedAfterWatch error
}

// statusAnswer is one answer of BuildStatus: a status, or an error.
type statusAnswer struct {
	status string
	err    error
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		project:    platform.Project{ID: "p-1", Name: "my-project"},
		validation: platform.ValidateResult{Valid: true},
		sourceKey:  "source-key-1",
		packageID:  "pkg-1",
		frames:     []string{"building", "Status - completed"},
		statuses:   []statusAnswer{{status: "completed"}},
		deployed: platform.DeployResult{
			InstanceID:  "i-1",
			ServiceName: "my-project-dev",
			HostURLs:    []string{"https://one.example.test", "https://two.example.test"},
		},
	}
}

// enter records a call and runs the hook before the method does anything else.
func (f *fakeAPI) enter(name string) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	hook := f.onCall
	f.mu.Unlock()
	if hook != nil {
		hook(name)
	}
}

func (f *fakeAPI) called(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, name)
}

// count is how many times the named call was made.
func (f *fakeAPI) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
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

func (f *fakeAPI) WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error {
	f.enter(callWatchBuild)
	f.mu.Lock()
	f.watchedID = packageID
	frames, blocks, err, late, final := f.frames, f.watchBlocks, f.watchErr, f.lateFrames, f.finalStatus
	f.mu.Unlock()
	for _, frame := range frames {
		onLine(frame)
	}
	if final != nil {
		select {
		case <-final:
			for _, frame := range late {
				onLine(frame)
			}
		case <-ctx.Done():
		}
	}
	if blocks {
		<-ctx.Done()
		err = ctx.Err()
	}
	f.mu.Lock()
	f.watchCtxErr = ctx.Err()
	f.mu.Unlock()
	return err
}

func (f *fakeAPI) BuildStatus(_ context.Context, _ string) (string, error) {
	f.enter(callBuildStatus)
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.statuses[min(f.statusCalls, len(f.statuses)-1)]
	f.statusCalls++
	if a.status == "completed" || a.status == "failed" {
		f.closeFinal.Do(func() {
			if f.finalStatus != nil {
				close(f.finalStatus)
			}
		})
	}
	return a.status, a.err
}

func (f *fakeAPI) DeployInstance(_ context.Context, req platform.DeployRequest) (platform.DeployResult, error) {
	f.enter(callDeployInstance)
	f.mu.Lock()
	f.deployReq = req
	f.deployedAfterWatch = f.watchCtxErr
	f.mu.Unlock()
	if f.deployErr != nil {
		return platform.DeployResult{}, f.deployErr
	}
	return f.deployed, nil
}

// tarEntries returns the names in the tar.gz at path, or nil when it is not one.
func tarEntries(path string) []string {
	raw, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer raw.Close()
	zr, err := gzip.NewReader(raw)
	if err != nil {
		return nil
	}
	defer func() { _ = zr.Close() }()
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
