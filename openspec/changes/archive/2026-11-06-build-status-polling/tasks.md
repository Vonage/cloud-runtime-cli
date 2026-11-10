# Tasks

Work in the worktree that publishing creates. It must be cut from the rewritten CLI, not from `main`, because the code below exists only there. Run every command from the worktree root. `make test` runs `go test -race ./...`; the contract targets need `VCR_CONTRACT=1`, which the Makefile sets.

## 1. Platform client: the status route

- [x] 1.1 Add `BuildStatus` and `IsTransportError` to the platform client.
  - Files: `internal/platform/reads.go` (new `routeBuildStatus = readRoute{label: "GET /packages/{id}/build/status"}`, `notFound` false, and `func (c *Client) BuildStatus(ctx context.Context, packageID string) (string, error)`), `internal/platform/upload_retry.go` (new exported `IsTransportError(err error) bool`: true for a `*url.Error` that is not a `*policyError` and not `isLocationFailure`, the test `retryReason` already makes; `retryReason` calls it), new test `internal/platform/buildstatus_test.go`.
  - Behaviour: `checkServerID(fieldPackageID, packageID)` first; GET `endpoint(c.base, "packages", packageID, "build", "status")` through `c.core.read`; decode `{"packageId","status"}`; return the status when it is `pending`, `building`, `completed` or `failed`, else an error naming the value.
  - Test first (table, against an `httptest` server as `reads_test.go` does): each of the four statuses comes back; an unknown status is an error; a 404 with the envelope is a `*clierr.APIError` with `HTTPStatus` 404 and `ServerCode` 2006 and is not `ErrNotFound`; a 404 without one carries the older-platform message naming `GET /packages/{id}/build/status`; an unsafe id makes no request. `IsTransportError`: true for a dial error, false for a `*clierr.APIError`, a `policyError`, a decode error and `context.Canceled`.
  - Verify: `go test ./internal/platform/ -run 'TestBuildStatus|TestIsTransportError|TestUpload' -count=1`
  - Done when: the tests pass and `go vet ./internal/platform/` is clean.

## 2. Platform client: the watch only streams

- [x] 2.1 Stop `WatchBuild` deciding the outcome.
  - Files: `internal/platform/watch.go`, `internal/platform/watch_frames_test.go`, `internal/platform/watch_fake_test.go`, `internal/platform/watch_context_test.go`, `internal/platform/watch_connect_test.go`, `internal/platform/watch_helpers_test.go`.
  - Behaviour: delete `frameStatus`, `statusPrefix`, `statusCompleted`, `statusFailed` and `watchFailed`. `followBuild` passes every frame to `onLine` and never ends on content. A normal close returns `nil`. The read-limit error, connect errors and reconnects after a drop stay as they are. A context end returns `ctxEnded(ctx)` unwrapped (`endedError` moves to task 4.1).
  - Test first:
    - Rewrite `TestWatchBuildEndsOnCompletedFrame`, `TestWatchBuildFailedFrames` and `TestWatchBuildIgnoresLookalikeFrames` into one `TestWatchBuildStreamsEveryFrameUntilClose`: frames including `ℹ Status - completed` and `Status - failed` are all delivered, and the call returns `nil` on the server's normal close.
    - Change `TestWatchBuildNormalClosure` to expect `nil`, and the normal-closure case of `TestReadFailureOrder` likewise.
    - Change `TestWatchBuildClosesTheConnectionWhenItEnds` to end the watch by cancelling its context after the frame (it now holds the connection open and would block until `waitLimit`).
    - Change the `wantCanceled` / `wantTimedOut` helpers in `watch_helpers_test.go`, and their uses in `watch_context_test.go` and `watch_connect_test.go`, to `errors.Is(err, context.Canceled)` / `errors.Is(err, context.DeadlineExceeded)`.
    - Delete `TestFrameStatus`. Keep the connect, reconnect and frame-limit tests.
  - Verify: `go test ./internal/platform/ -run 'TestWatchBuild|TestReadFailureOrder|TestCtxEnded|TestWatchError|TestIsFinal' -count=1`
  - Done when: the tests pass and `grep -n "Status - " --include='*.go' --exclude='*_test.go' -r internal/platform` prints nothing.

## 3. Fake platform: serve the status route

- [x] 3.1 Add the route and its fixture.
  - Files: `test/fakeplatform/fixtures.go` (new `BuildStatus []StatusAnswer \`yaml:"buildStatus"\`` on `Fixtures`, `StatusAnswer{Status string; HTTP int}`, validation that each sets exactly one, and a default of `[{status: completed}]` in `Defaults()`), `test/fakeplatform/reads.go` (route `GET /packages/{id}/build/status` added to `readRoutes`, answering the n-th request with the n-th answer, the last repeating; an `HTTP` answer uses the error envelope; an id other than the deploy fixture's `PackageID` answers 404 with code 2006 `package not found`), `test/fakeplatform/envelope.go` (`codePackageNotFound = 2006`), `test/fakeplatform/world.go` (a request counter).
  - Test first: `test/fakeplatform/reads_test.go` gains `TestBuildStatusAnswersInOrder` (statuses in order, then the last repeated; an `HTTP: 503` answer uses the error envelope) and `TestBuildStatusUnknownPackage` (404, code 2006). `fixtures_test.go` gains a case where an answer sets both fields or neither.
  - Verify: `go test ./test/fakeplatform/ -count=1`
  - Done when: the tests pass.

## 4. Deploy: the status poll decides

- [x] 4.1 Add the poll loop and wire it into the build stage.
  - Files:
    - `internal/deploy/deploy.go`: add `BuildStatus(ctx context.Context, packageID string) (string, error)` to `API`.
    - New `internal/deploy/build.go`: `awaitBuild`, the constants `pollInterval = 2 * time.Second` and `streamGrace = 5 * time.Second`, and `endedError` moved from `internal/platform/watch.go` with identical texts.
    - `internal/deploy/run.go`: `Run(ctx, t, api, clk clock.Clock, p)`; the runner keeps `clk`.
    - `internal/deploy/stages.go` (`watchBuild`) and `internal/deploy/rich.go` (`watchBuildRich`).
    - `internal/cli/deploy/deploy.go`: pass `env.Clock`.
  - Behaviour, per `specs/deploy-build-outcome/spec.md` and design decisions 1, 4, 5 and 6:
    - Start the watch in a goroutine under a child context, printing frames as today. Poll `BuildStatus` at once, then every `pollInterval` through `clk.Sleep`.
    - On `completed` or `failed`: wait for the watch goroutine to return or for `streamGrace` to pass (timed with `clk.Sleep` in its own goroutine under a cancellable context), whichever is first. If the grace passed, cancel the child context. Then wait for the watch goroutine to return in every case, before printing the package-built line or returning. `failed` then returns `error while building package <id>`.
    - A watch that returns `nil` says nothing. A watch that returns a non-context error prints `<Style.WarningIcon()> build output is unavailable: <reason>` once to stderr, and polling goes on.
    - Retry a `*clierr.APIError` with a 5xx `HTTPStatus` or an error for which `platform.IsTransportError` is true on the next tick, and return the third in a row. Every other error returns at once.
    - A context end returns `endedError(packageID, err)`, where `err` is `ctx.Err()`, or `context.DeadlineExceeded` when that is nil but `ctx.Deadline()` has passed.
    - Every error keeps the `failed to watch deployment for package_id=%q: %w` wrapper.
  - Test first:
    - `internal/deploy/fake_api_test.go`: give `fakeAPI` a mutex around `calls` and every field the build stage reads or writes; a `statuses []statusAnswer` (status or error, the last repeating); a `BuildStatus` method; a `callBuildStatus` constant; and a `watchBlocks bool` that makes `WatchBuild` send its frames and then block until its context ends.
    - The harnesses (`deploy_test.go` `newHarness`, `rich_test.go` `newRichRun`, `rich_match_test.go`) pass `Run` a `clocktest.Fake` of their own, separate from the one given to `term.TestRich`, so the poll's sleeps do not move the rich step durations.
    - New `internal/deploy/build_status_test.go`, one test per spec scenario:
      - `pending`, `building`, then `completed` deploys after three `BuildStatus` calls.
      - `failed` exits with the build error and makes no `DeployInstance` call.
      - A `Status - failed` frame with `completed` deploys.
      - A watch that returns `nil` early, then `completed`, deploys with empty stderr.
      - A watch error prints the warning once and deploys.
      - `watchBlocks` with `completed` deploys, and the watch's context was cancelled before `DeployInstance` was called.
      - Frames sent before the watch returns all appear before the package-built line.
      - 503 once then `completed` deploys; 503 three times fails with it.
      - A decode-style error fails at once with one `BuildStatus` call.
      - An envelope 404 fails with HTTP status 404 and code 2006 in the error.
      - A cancelled context gives the canceled text.
      - The poll's clock records `pollInterval` between calls (`Sleeps()`).
    - Update the existing tests that change:
      - `TestRunWatchFailure` in `build_test.go`: a watch error now deploys with a warning.
      - The "build watch" case in `rich_match_test.go`: it now sets a `failed` status instead of `watchErr`.
      - The exact call lists in `deploy_test.go` and `rich_test.go`: assert the calls before `callCreatePackage` and after it (`callDeployInstance`) in order, and the build-stage calls (`callWatchBuild` once, `callBuildStatus` one or more times) as a set.
      - Every `Run(...)` caller: `deploy_test.go`, `rich_test.go`, `rich_match_test.go`.
    - `internal/cli/deploy/fake_test.go` (`TestDeployAgainstTheFakePlatform`) and `internal/cli/deploy/rich_test.go` (`TestRichDeployAgainstTheFakePlatform`): the request list asserts the requests up to `POST /packages` in order, then `GET /packages/<id>/build/status` and `GET /packages/<id>/build/watch` in either order, then `POST /deployments` last. This needs the fake route from task 3.1.
  - Verify: `go test -race ./internal/deploy/ ./internal/cli/deploy/ -count=1`
  - Done when: the tests pass and `go build ./...` succeeds.

## 5. Contract harness and scenarios

- [x] 5.1 Count the status route as a read.
  - Files: `test/contract/reads.go` (`isReadRoute` also accepts `/packages/<id>/build/status`; update the "nine read routes" comments to ten), `test/contract/requests_test.go`.
  - Test first: extend `TestReadsFilterRemovesTheNineRoutes` (rename it `TestReadsFilterRemovesTheReadRoutes`) with `euw1 GET /packages/<uuid>/build/status`, and a case showing `euw1 GET /packages/<uuid>/build/logs` is kept.
  - Verify: `go test ./test/contract/ -run 'TestReadsFilter' -count=1`
  - Done when: the test passes.
- [x] 5.2 Update every scenario that reaches the build, and the allowed diffs.
  - Scenarios: every scenario whose transcript in `test/contract/testdata/transcripts/` has a `WS /packages/<uuid>/build/watch` block, found with `grep -l "WS /packages/<uuid>/build/watch" test/contract/testdata/transcripts/*`. That is the `deploy-*` and `security-build-*` ones that build, plus `security-escape-build-frames`, `security-redirect-upload-307`, `security-upload-env-negation-brings-back-env`, `security-upload-git-file-is-kept`, `security-upload-git-negation-brings-back-git`, `security-upload-skips-git-and-env-files` and `security-upload-symlink-outside-project-is-a-link-only`.
  - Changes:
    - Each of those scenarios adds the status GETs it makes to `expect-reads` (`"euw1 GET /packages/<uuid>/build/status"`, once per poll; one in the default case, where the fixture answers `completed`).
    - `deploy-build-failed.yaml` gets `buildStatus: [{status: failed}]`.
    - `deploy-build-timeout.yaml` and `deploy-build-interrupted.yaml` get `buildStatus: [{status: building}]` and one status read. Keep `deploy-build-timeout`'s `-t 2s`. The deadline starts before the upload and the polls start at package creation, so the second poll always falls after the deadline. Keep `deploy-build-interrupted` signalling right after `Starting build`. If either flakes over 10 runs, stop and report rather than loosening the read check.
    - `deploy-build-normal-close-before-completion.yaml` gets a new description: "a build watch that closes before the build completed only loses output".
    - Rename `security-build-frame-exact-failure-line-ends-the-watch` to `security-build-frame-failure-line-does-not-end-the-build`, and `security-build-frame-watch-failure-line-ends-the-watch` to `security-build-frame-watch-failure-line-does-not-end-the-build`, with matching `testdata/transcripts` file names. The `security-build-*` glob in `coverage.txt` still matches them.
    - `test/contract/allowed-diffs.txt`: add `stdout`, `stderr`, `exit` and `requests` lines with the reason "the build status route decides the outcome; the build log stream only shows output" for `deploy-build-normal-close-before-completion`, `deploy-build-connect-always-rejected` and the two renamed scenarios (by their new ids). Change the reason on the `security-build-frame-with-status-word-does-not-end-the-watch` line to the same text.
  - Test first: run the verify commands below before editing and note the failures. Each must be one of the scenarios listed here.
  - Verify: `make contract NO_GRAPHQL=1 SCENARIO='deploy-*'`, then `make contract NO_GRAPHQL=1 SCENARIO='security-*'`, then `go test ./test/contract/ -run 'TestCoverage|TestScenario|TestAllowed' -count=1`
  - Done when: all three pass.
- [x] 5.3 Re-record and review the rich goldens that changed.
  - Files: `test/contract/testdata/rich/deploy-on-terminal.txt` and any other rich golden whose scenario reaches the build.
  - Verify: `make contract-rich-record SCENARIO='deploy-*'`, review `git diff test/contract/testdata/rich/` (only the build stage may change, and the build lines must still appear before `✓ Package built`), then `make contract-rich`
  - Done when: `make contract-rich` passes and the diff has been read.

## 6. Whole-repo checks

- [x] 6.1 Run everything.
  - Verify: `make test && make lint && make contract NO_GRAPHQL=1 && make contract-rich`
  - Done when: all pass. Record their output in the PR description.
- [x] 6.2 Check by hand against a development environment, with a config whose endpoint points at it:
  - `vcr deploy` of a small Node app builds and deploys, the build lines show, and stderr is empty.
  - A deploy whose `build-script` exits 1 fails with `error while building package <id>`.
  - Ctrl-C during the build ends with the canceled message.
  - Done when: all three behave as described. Note the result in the PR description, without hostnames.
