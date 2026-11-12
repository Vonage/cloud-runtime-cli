# deploy-progress Specification

## Purpose
How `vcr deploy` shows the image build and the deployed instance on a terminal: the build phases, the build script's
output, the log of a failed build, and the closing summary. Classic output is not covered here and does not change.

## Requirements

### Requirement: Frames are read by their opening words
In the rich and static presentations the CLI SHALL read each build-watch frame after removing its escape sequences
and one leading `ℹ ` (the info glyph and a space), and SHALL decide a frame's phase and kind only from its opening
words. A frame's content SHALL NOT end or fail the build.

#### Scenario: the info glyph is not part of the words
- **WHEN** the stream sends `\u001b[35mℹ\u001b[0m Retrieving image manifest x`
- **THEN** the frame is read as `Retrieving image manifest x`

### Requirement: Build phases
Before the first frame the phase SHALL be Waiting for build to start; any first frame SHALL set Preparing source.
Later frames SHALL move the phase only through the phase markers, and the phase SHALL never move back.

#### Scenario: an unknown frame does not move the phase back
- **WHEN** the stream sends `ℹ Pushing image to vcr.registry/x:1` and then `ℹ Retrieving image manifest y`
- **THEN** the phase stays Pushing image

### Requirement: Phase markers
These opening words SHALL set a phase: `Retrieving image`: Pulling base image. `Unpacking rootfs`, `Initializing
snapshotter`: Mounting filesystem. `WORKDIR `, `COPY `: Copying source. `Cmd: /bin/sh`: Running build script.
`Taking snapshot of full filesystem`: Saving image once the script ran, else Mounting filesystem. `Pushing image
to`, `Pushed `: Pushing image. While the build script runs only `Taking snapshot of full filesystem` SHALL move the
phase; another marker there is script output.

#### Scenario: the phases of a build
- **WHEN** the stream sends `ℹ Instance source code initialized successfully`, `ℹ Retrieving image manifest x`,
  `ℹ Unpacking rootfs as cmd COPY . . requires it.`, `ℹ COPY . .`, `ℹ Cmd: /bin/sh`, `ℹ added 67 packages`,
  `ℹ Taking snapshot of full filesystem...`, `ℹ Pushing image to vcr.registry/x:1`
- **THEN** the phase is, after each frame in turn, Preparing source, Pulling base image, Mounting filesystem,
  Copying source, Running build script, Running build script, Saving image, Pushing image

#### Scenario: a marker inside the build script
- **WHEN** the stream sends `ℹ Cmd: /bin/sh` and then `ℹ Pushed 3 files`
- **THEN** `Pushed 3 files` is build script output and the phase stays Running build script

### Requirement: Build script output
The build script's output SHALL be the frames after a `Cmd: /bin/sh` frame and before the next `Taking snapshot of
full filesystem` frame, except frames starting with `Util.Lookup returned:`, `Performing slow lookup of group ids`
or `Status - `. Frames starting with `Status - ` SHALL never be shown while the build runs.

#### Scenario: script lines among Kaniko lines
- **WHEN** the stream sends the frames of the "phases of a build" scenario
- **THEN** the only build script output is `added 67 packages`

#### Scenario: a lookup line inside the script
- **WHEN** the stream sends `ℹ Cmd: /bin/sh`, `ℹ Util.Lookup returned: &{Uid:1000}`, `ℹ npm ci`
- **THEN** the build script output is `npm ci` alone

### Requirement: The build step shows its phase
In the rich presentation the live line of the build step SHALL read `Building package`, then a bar of 7 segments
with one segment filled per phase reached after Waiting for build to start, then `<n>/7 <phase>`, then the elapsed
time as other live lines show it. On a terminal narrower than 60 columns the bar SHALL be left out. When the build
succeeds the step SHALL resolve to `✓ Package built` with its duration, as today.

#### Scenario: waiting for the build
- **WHEN** no frame has arrived yet
- **THEN** the live line shows the empty bar and `0/7 Waiting for build to start`

#### Scenario: during the build script
- **WHEN** the phase is Running build script on an 80-column terminal
- **THEN** the live line shows 5 of 7 segments filled and `5/7 Running build script`

### Requirement: A rolling window of script output
In the rich presentation the CLI SHALL show the last 8 lines of build script output under the live line, each
prefixed with a dim `  │ ` and cut to the terminal width with a trailing `…`, and no other frame while the build
runs. When the build succeeds the CLI SHALL erase those lines, so the screen keeps only the step's resolved line.

#### Scenario: a successful build on a terminal
- **WHEN** the build script prints 20 lines and the status route answers `completed`
- **THEN** the final screen has `✓ Package built` right after `✓ Package created` and none of the 20 lines

#### Scenario: a long script
- **WHEN** the build script has printed 12 lines and the build is still running
- **THEN** the screen shows lines 5 to 12 under the live line

### Requirement: Script output in the static presentation
In the static presentation the CLI SHALL print each line of build script output as it arrives, prefixed with
`  │ ` and not cut, and print no other frame while the build runs.

#### Scenario: static presentation
- **WHEN** `VCR_UI=static` and the build script prints 3 lines among 30 Kaniko lines
- **THEN** stdout shows the 3 lines, each prefixed with `  │ `, and no Kaniko line

### Requirement: A failed build shows the whole log
When the build status route answers `failed` or fails itself (an error it does not retry, or 3 in a row), or
`--timeout` ends the wait, the CLI SHALL print, before the error, every frame it did not leave on the screen, in
arrival order, each prefixed with `  │ `. In the rich presentation that is every frame; in the static presentation
every frame it did not already print. An interrupt (Ctrl-C) SHALL erase the build script lines and print no log.

#### Scenario: a build that fails
- **WHEN** the stream sends Kaniko lines and the script's `npm ERR! missing script: build`, and the status route
  answers `failed`
- **THEN** the screen shows every frame, Kaniko lines included, in arrival order, then the build error, and the
  exit code is 1

#### Scenario: the status route fails
- **WHEN** the stream sends Kaniko and script lines, and the status route answers 503 three times in a row
- **THEN** the screen shows every frame in arrival order, then the error

#### Scenario: Ctrl-C during the build
- **WHEN** the user presses Ctrl-C while the build script runs
- **THEN** no build log is printed and the deploy ends with the canceled message

### Requirement: The deployed instance box
In the rich and static presentations the deploy summary SHALL start with a blank line and a box drawn with
`╭ ╮ ╰ ╯ ─ │`, whose top border carries the title `✓ <project name> is live`. The box SHALL hold the rows `URL`
(one row per host address, the label on the first only), `Instance` (the instance id) and `Service` (the service
name), with the labels padded to one width.

#### Scenario: one host address
- **WHEN** a deploy succeeds with one host address on a 100-column terminal
- **THEN** the box shows the title, the `URL` row with the whole address, then the `Instance` and `Service` rows

### Requirement: No literal summary lines on a terminal
In the rich and static presentations the lines `Instance has been deployed!`, `Instance id:`, `Instance service
name:`, `Instance host address:` and `Webhook <path>:` SHALL NOT appear.

#### Scenario: a deploy on a terminal
- **WHEN** a deploy with webhooks succeeds in the rich presentation
- **THEN** stdout has none of those lines

### Requirement: The application webhooks box
When the deploy response's applied block has at least one `address` string field, the summary SHALL add a box
titled `Application webhooks` right after the instance box. It SHALL hold one group per capability, the first
segment of the address's dotted path, in path order, each starting with `● <capability>`. Without an address the
box SHALL be left out, and the not-applied warning on stderr stays as it is.

#### Scenario: no webhooks
- **WHEN** the deploy response has no `applicationCapabilities`
- **THEN** the summary has the instance box only

### Requirement: Webhook rows
Each webhook row SHALL show the name (the rest of the dotted path, without a leading `webhooks.` and a trailing
`_url`), the `http_method` string beside that `address` or a blank when there is none, and the whole address, in
columns each padded to one width across the box.

#### Scenario: voice and messages
- **WHEN** the applied block has `voice.webhooks.answer_url` (POST, `https://h/voice/answer`) and
  `messages.webhooks.inbound_url` (POST, `https://h/messages/inbound`)
- **THEN** the box shows `● messages` with the row `inbound  POST  https://h/messages/inbound`, then `● voice` with
  the row `answer   POST  https://h/voice/answer`, the name column padded to the width of `inbound`

#### Scenario: a webhook without a method
- **WHEN** the applied block has `video.webhooks.archive_status.address` and no `http_method` beside it
- **THEN** the `archive_status` row has a blank method and the address

### Requirement: Boxes fit the terminal
A box SHALL be as wide as its widest line. When a box's widest line does not fit the terminal width, the summary
SHALL print the same titles and rows without borders, indented two spaces. Addresses and URLs SHALL never be cut.
Under `NO_COLOR` the summary SHALL contain no escape sequence.

#### Scenario: a narrow terminal
- **WHEN** the host address is 70 characters long and the terminal is 60 columns wide
- **THEN** the summary has no border characters and shows the whole address

### Requirement: The debugger box
In the rich and static presentations `vcr debug` SHALL show, where it showed the `Debugger ready` block, a blank
line and a box titled `✓ Debugger ready` with the rows `URL` (the debug host) and `Service` (the service name),
then the application webhooks box with the same groups and rows as `vcr deploy`, then the forwarding notice as
today. The boxes SHALL fit the terminal as deploy's do, and the `Webhook <path>` rows SHALL NOT appear.

#### Scenario: a debug session with webhooks
- **WHEN** a debug session is ready and its applied block has `voice.webhooks.answer_url` (POST)
- **THEN** stdout shows the `✓ Debugger ready` box with `URL` and `Service`, then `Application webhooks` with
  `● voice` and the row `answer  POST  <address>`, then the forwarding notice

### Requirement: Classic output is unchanged
With the classic presentation (no terminal, `CI` set, or `VCR_UI=classic`), `vcr deploy` and `vcr debug` SHALL print
every build frame and the summary exactly as before this change.

#### Scenario: deploy in CI
- **WHEN** `vcr deploy` runs with `CI=true` and the build sends Kaniko lines
- **THEN** stdout holds every frame and the `Instance has been deployed!` summary, byte-identical to the previous release
