# Spec Delta

## Purpose
Defines `vcr app capabilities add`, which writes default Vonage application capabilities into the manifest's
`application-capabilities` block, and how it edits the file.

## ADDED Requirements

### Requirement: One flag per capability with fixed defaults
`vcr app capabilities add` SHALL accept `--voice`, `--messages`, `--rtc`, `--verify`, `--video`, `--vbc`,
`--meetings` and `--network-apis`, and write the default below for each flag given. A webhook address SHALL be
`{{instance_url}}/<capability>/<webhook name without _url, with - for _>`. Every webhook except video's SHALL have
`http_method: POST`; video webhooks SHALL have `active: true` and no `http_method`.

#### Scenario: Voice default
- **WHEN** `vcr app capabilities add --voice` runs on a manifest with no block
- **THEN** the manifest gains `voice.webhooks.answer_url` at `{{instance_url}}/voice/answer` and
  `voice.webhooks.event_url` at `{{instance_url}}/voice/event`, both `POST`, and the command exits 0

#### Scenario: Messages default
- **WHEN** `vcr app capabilities add --messages` runs
- **THEN** `messages` is `version: v1` with `inbound_url` at `/messages/inbound` and `status_url` at `/messages/status`

#### Scenario: RTC default
- **WHEN** `vcr app capabilities add --rtc` runs
- **THEN** `rtc.webhooks.event_url` is at `{{instance_url}}/rtc/event`

#### Scenario: Verify default
- **WHEN** `vcr app capabilities add --verify` runs
- **THEN** `verify` is `version: v2` with `status_url` at `{{instance_url}}/verify/status`

#### Scenario: Video default
- **WHEN** `vcr app capabilities add --video` runs
- **THEN** `video.webhooks` has `session_created`, `session_destroyed`, `connection_created`, `connection_destroyed`,
  `stream_created` and `stream_destroyed`, and `stream_created` is `{address: "{{instance_url}}/video/stream-created", active: true}`

#### Scenario: VBC default
- **WHEN** `vcr app capabilities add --vbc` runs
- **THEN** `vbc` is an empty mapping

#### Scenario: Meetings default
- **WHEN** `vcr app capabilities add --meetings` runs
- **THEN** `meetings.webhooks` has `recording_changed`, `room_changed` and `session_changed`, and `room_changed` is
  at `{{instance_url}}/meetings/room-changed`

#### Scenario: Network APIs default
- **WHEN** `vcr app capabilities add --network-apis --network-application-id net-1 --redirect-uri https://example.com/cb` runs
- **THEN** `application-capabilities.network_apis` is `{network_application_id: net-1, redirect_uri: "https://example.com/cb"}`

### Requirement: Flag errors are reported before the file is touched
The command SHALL exit 1 without changing the manifest when no capability flag is given, when `--network-apis` is
given without both `--network-application-id` and `--redirect-uri`, or when either of those two is given without
`--network-apis`.

#### Scenario: No capability flag
- **WHEN** `vcr app capabilities add` runs with no capability flag
- **THEN** it exits 1 with an error naming the capability flags, and the manifest is unchanged

#### Scenario: Network APIs without its values
- **WHEN** `vcr app capabilities add --network-apis --redirect-uri https://example.com/cb` runs
- **THEN** it exits 1 with an error naming `--network-application-id`, and the manifest is unchanged

### Requirement: Existing capabilities are kept unless forced
A capability already present under `application-capabilities` SHALL be left as written and reported on stderr as
skipped, unless `--force` is given, in which case it SHALL be replaced by the default. Capabilities not named by a
flag SHALL never change.

#### Scenario: Skip an existing capability
- **WHEN** the block has `voice` with an `answer_url` of `{{instance_url}}/my-answer` and `vcr app capabilities add --voice --messages` runs
- **THEN** `voice` is unchanged, `messages` is added, stderr says `voice` was skipped and names `--force`, and the command exits 0

#### Scenario: Force replaces only the named capability
- **WHEN** the block has `voice` and `rtc`, and `vcr app capabilities add --voice --force` runs
- **THEN** `voice` is the default and `rtc` is unchanged

### Requirement: The capabilities list is replaced by the block
When the command writes the block and the manifest has `instance.capabilities`, it SHALL remove that key and say on
stderr which entries it removed.

#### Scenario: Migrate from the list
- **WHEN** the manifest has `instance.capabilities: [voice, messages-v1]` and `vcr app capabilities add --voice --messages` runs
- **THEN** `instance.capabilities` is gone, the block has `voice` and `messages`, and stderr names `voice` and `messages-v1` as removed

### Requirement: The rest of the manifest is kept as written
The command SHALL change only the `application-capabilities` block and, when present, `instance.capabilities`.
Comments, key order, quoting and keys the CLI does not know SHALL stay as they were.

#### Scenario: Comments and unknown keys survive
- **WHEN** the manifest has a comment above `instance:` and a top-level key `custom: 1`, and `vcr app capabilities add --rtc` runs
- **THEN** the written manifest still has the comment in the same place and `custom: 1`

### Requirement: The manifest is found like deploy finds it
The command SHALL write to the manifest named by `--filename`, or else the one `vcr deploy` would pick in the
current directory (the newest of the manifest file names it accepts). When there is none, it SHALL
exit 1 with an error saying the manifest file was not found and pointing at `--print`.

#### Scenario: No manifest
- **WHEN** `vcr app capabilities add --voice` runs in a directory with no manifest and no `--filename`
- **THEN** it exits 1 with an error containing `manifest file not found` and `--print`, and creates no file

### Requirement: Print instead of writing
With `--print`, the command SHALL write the resulting `application-capabilities` block as YAML to stdout and leave
every file unchanged. It SHALL need no manifest: without one, the block holds only the requested defaults.

#### Scenario: Print with no manifest
- **WHEN** `vcr app capabilities add --rtc --print` runs in a directory with no manifest
- **THEN** stdout is an `application-capabilities` block with only `rtc`, the command exits 0, and no file is created

### Requirement: The command works offline
The command SHALL make no network request and SHALL not need CLI credentials or a configured region.

#### Scenario: No config file
- **WHEN** `vcr app capabilities add --vbc` runs with no CLI config file
- **THEN** it exits 0 and sends no request
