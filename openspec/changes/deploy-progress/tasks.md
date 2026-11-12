# Tasks

`<WT>` is this worktree, on the branch that already carries the `application-capabilities` and
`capabilities-generator` changes. Every task works test first: write the test, run it and watch it fail for the
right reason, then write the code. Every task ends with `cd <WT> && go test ./<package>/...` passing for the
packages it touched, and every group ends with `cd <WT> && make test lint` passing before its commit. Classic
output must not change: `cd <WT> && make contract` passes at the end of every group.

## Exact strings

- Live label: `Building package`; progress text `<n>/7 <phase>`
- Phases, in order: `Waiting for build to start`, `Preparing source`, `Pulling base image`, `Mounting filesystem`,
  `Copying source`, `Running build script`, `Saving image`, `Pushing image`
- Bar: 7 segments of `━━` (filled) and dim `──` (empty), two spaces either side; left out under 60 columns
- Tail and log prefix: dim `  │ `; tail size 8; log cap 2,000 frames, then dim `… <n> earlier lines` first
- Instance box title: `✓ <project name> is live`; rows `URL`, `Instance`, `Service`
- Webhooks box title: `Application webhooks`; group head `● <capability>`
- Box characters: `╭ ╮ ╰ ╯ ─ │`; a title sits in the top border as `╭─ <title> ─…─╮`; one blank row inside the box
  above and below the rows; two spaces of padding left and right

## 1. Build-log reader

- [x] 1.1 Failing table test in new `internal/deploy/buildlog_test.go` for `buildLog.add(frame string) frameKind`
  (kinds: hidden, script, status) and `buildLog.phase() int`: every scenario of the "Frames are read by their
  opening words" requirement, plus: escape sequences and one leading `ℹ ` are removed before matching; a second
  `Cmd: /bin/sh` after a snapshot makes later lines script output again while the phase stays; `frames()` returns
  every frame in arrival order without its `ℹ `; past 2,000 frames it keeps the last 2,000 and reports the dropped
  count. Verify: `cd <WT> && go test ./internal/deploy/ -run TestBuildLog` fails.
- [x] 1.2 Implement `internal/deploy/buildlog.go`: the phase names, the prefix table from the spec in one place, the
  forward-only phase, the script window flag and the bounded record. Verify: the 1.1 test passes.

## 2. Live region: phase bar and tail

- [x] 2.1 Failing tests in `internal/term/live_test.go` and `internal/term/surface_test.go`, using `term.TestRich`
  and the fake clock: `Step.SetProgress(5, 7, "Running build script")` draws `<glyph> Building package  <bar>  5/7
  Running build script` with 5 filled segments at 80 columns, and no bar at 59 columns; `Step.Tail` with 12 lines
  draws the last 8 under the live row, each prefixed with `  │ ` and cut to width−1 with `…`; a write to stdout
  while a tail is drawn leaves the screen (`TermBuffer.Screen()`) with the written text, then the live row and the
  tail, and no leftover rows; `Stop` and `OK` erase every row of the region. Static: both methods draw nothing.
  Verify: `cd <WT> && go test ./internal/term/ -run 'TestLive|TestSurface'` fails.
- [x] 2.2 Implement in `internal/term/step.go`, `live.go` and `surface.go`: the progress and tail fields under the
  step's lock; the surface tracks how many rows it drew and erases them with `\r`, `\x1b[<n>A` and `\x1b[J`
  before every write and on finish; rows re-cut to `columns()` on each draw. Verify: the 2.1 tests and
  `cd <WT> && go test ./internal/term/...` pass, including the existing live and surface tests.

## 3. The build step on a terminal

- [ ] 3.1 Failing tests in `internal/deploy/rich_test.go` with `newRichRun`: (a) Rich, Kaniko frames then 20 script
  lines then `completed`: the final stdout screen goes from `✓ Package created` straight to `✓ Package built`, and
  no frame is on stdout or stderr; (b) mid-build (fake clock, the stream held open): the stderr screen shows the
  live row with `5/7 Running build script` and the last 8 script lines; (c) `failed`: before the error, stdout has
  every frame in arrival order under `  │ `, Kaniko lines included, and the exit error is unchanged; (d) timeout:
  the same log; (e) canceled context: no log; (f) Static: script lines printed as they arrive, no Kaniko line, and on
  `failed` only the frames not printed yet. Update the existing gutter tests that expect every frame. Verify:
  `cd <WT> && go test ./internal/deploy/ -run TestRich` fails for the new cases.
- [ ] 3.2 Implement in `internal/deploy/rich.go` `watchBuildRich`: feed each frame to `buildLog`, update
  `SetProgress` and `Tail` (Rich) or write script lines to the gutter (Static); on an error other than
  `context.Canceled` stop the step and write the held frames to the gutter before returning. `build.go` stays as
  it is. Change `TestStaticDeployMatchesRichScreen` in `rich_match_test.go` to drop the static gutter lines before
  comparing. Verify: the 3.1 tests and `cd <WT> && go test ./internal/deploy/...` pass.

## 4. Webhook methods

- [ ] 4.1 Failing test in `internal/platform/appcaps_test.go`: `WebhookAddresses` sets `Method` from the string
  `http_method` beside `address`, blank when absent or not a string; paths and order unchanged. Verify:
  `cd <WT> && go test ./internal/platform/ -run TestWebhookAddresses` fails.
- [ ] 4.2 Add `Method` to `WebhookAddress` in `internal/platform/appcaps.go`. Classic and debug output stay as they
  are. Verify: the 4.1 test and `cd <WT> && go test ./internal/platform/... ./internal/debugsession/...` pass.

## 5. Boxes

- [ ] 5.1 Failing tests in new `internal/term/box_test.go` with `term.TestRich` at fixed widths: a titled box with
  padded labels sized to its widest row; a box with groups (a head row and indented rows); a value with escape
  sequences measured without them; a widest line over the width gives the borderless form with the same rows and
  no border character; under `NO_COLOR` no escape sequence; Classic prints nothing. Add `Style.Magenta` with a test
  in `style_test.go`. Verify: `cd <WT> && go test ./internal/term/ -run 'TestBox|TestStyle'` fails.
- [ ] 5.2 Implement `internal/term/box.go` and `Style.Magenta` in `style.go`. Verify: the 5.1 tests pass.

## 6. The deploy summary

- [ ] 6.1 Failing tests in `internal/deploy/rich_test.go` and `appcaps_test.go` for Rich and Static: the instance box
  rows and title from the spec; two host URLs give two `URL` rows with the label on the first; the webhooks box from
  the spec's voice and messages scenario with `POST`; a video address with a blank method; no webhooks box without
  addresses; no `Instance has been deployed!` or `Instance …:` line; the not-applied warning still on stderr. Update
  `TestRichSummaryListsTheAppliedWebhookAddresses` and the other summary expectations. Verify:
  `cd <WT> && go test ./internal/deploy/ -run 'TestRich|TestDeploy'` fails for the new cases.
- [ ] 6.2 Rewrite `summaryRich` in `internal/deploy/rich.go` with `term.Box`: title from the plan's project name,
  webhooks grouped by the first path segment, names without `webhooks.` and `_url`, bullet colours voice magenta,
  messages blue, rtc yellow, video cyan, verify green, any other bold. `summary.go` classic branch untouched.
  Verify: the 6.1 tests and `cd <WT> && go test ./internal/deploy/...` pass.

## 7. Contract and docs

- [ ] 7.1 New rich scenarios in `test/contract/scenarios/`: `deploy-build-phases-on-terminal.yaml` (Kaniko frames
  from the spec scenario plus script lines, `completed`), `deploy-build-fails-on-terminal.yaml` (status `failed`)
  and `deploy-webhooks-on-terminal.yaml` (a block whose response has voice and messages addresses), each
  `presentation: rich`. Record them and re-record `deploy-on-terminal` with
  `cd <WT> && make contract-rich-record SCENARIO=<id>`; read every new golden and check it against the spec.
  Verify: `cd <WT> && make contract contract-rich` passes and `git diff --stat test/contract/testdata/transcripts`
  is empty.
- [ ] 7.2 README "Output" table: `VCR_UI=static` keeps the build script lines it prints; one sentence that `vcr
  deploy` on a terminal shows the build phase and only the build script's output, and the full log when a build
  fails. Verify: `cd <WT> && go test ./internal/term/ -run TestReadme` passes.

## 8. Checks

- [ ] 8.1 `cd <WT> && make build test lint tidy contract contract-rich` passes, and the leak check prints nothing
  before each commit. Install the build for a manual run of `vcr deploy` against a test project: the live line, the
  collapse and the boxes.

## Workflow follow-up

- `openspec validate deploy-progress --strict`, review, then archive as the last commit.
