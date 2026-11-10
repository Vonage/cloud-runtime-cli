# deploy-build-outcome Specification

## Purpose
Defines how `vcr deploy` learns that a package build has finished and whether it succeeded, and what the build log stream is used for.

## Requirements

### Requirement: Polling the build status
After creating a package, `vcr deploy` SHALL poll `GET /<version>/packages/<id>/build/status` on the region's deployment API every 2 seconds, starting immediately, until it answers `completed` or `failed`. `pending` and `building` SHALL keep it waiting. Any other value SHALL end the deploy with exit code 1 and an error naming the value.

#### Scenario: a build still in progress
- **WHEN** the status route answers `pending`, then `building`, then `completed`
- **THEN** the CLI asks three times, 2 seconds apart, and then continues

### Requirement: The build status route decides the outcome
`completed` SHALL continue the deploy. `failed` SHALL end it with exit code 1 and the error `failed to watch deployment for package_id="<id>": error while building package <id>`.

#### Scenario: a build that succeeds
- **WHEN** the status route answers `building`, then `completed`
- **THEN** the deploy shows the package as built and deploys the instance

#### Scenario: a build that fails
- **WHEN** the status route answers `failed`
- **THEN** the deploy exits 1 with `error while building package <id>` and does not deploy the instance

#### Scenario: a status line in the stream does not decide
- **WHEN** the build log stream sends `Status - failed` while the status route answers `completed`
- **THEN** the deploy continues and deploys the instance

### Requirement: The build log stream only shows output
The CLI SHALL keep showing the build-watch stream's frames as build output, and SHALL NOT end or fail the build because of any frame's content. When the server closes the stream normally, the CLI SHALL say nothing and keep waiting on the status route.

#### Scenario: the stream closes before the status is final
- **WHEN** the stream sends one line and closes normally, and the status route then answers `completed`
- **THEN** the deploy succeeds, the line is shown before the package-built line, and stderr is empty

### Requirement: The stream ends before the deploy goes on
When the status becomes final while the stream is still open, the CLI SHALL wait up to 5 seconds for the stream to end on its own, then close it. The CLI SHALL have stopped reading the stream before it prints the package-built line or any later output.

#### Scenario: a stream that stays open
- **WHEN** the status route answers `completed` and the stream neither sends nor closes
- **THEN** the CLI closes the stream after 5 seconds and continues the deploy

#### Scenario: output that arrives after the status
- **WHEN** the status route answers `completed` and the stream then sends a line and closes within 5 seconds
- **THEN** that line is shown before the package-built line

### Requirement: Stream failures do not fail the deploy
A build-watch stream that cannot connect, or whose frame exceeds the size limit, SHALL NOT fail the deploy. The CLI SHALL print one line to stderr, the warning icon followed by `build output is unavailable: <reason>`, and keep waiting on the status route. A stream that drops without a close frame SHALL still be reconnected as before.

#### Scenario: the stream never connects
- **WHEN** every connection attempt to the stream is refused and the status route answers `completed`
- **THEN** the deploy succeeds, and stderr shows the build output warning once

### Requirement: Status route errors
A 404 from the status route with the platform's error envelope SHALL end the deploy with exit code 1, showing the platform's error (HTTP status 404 and its code). A 404 without the envelope SHALL end it with the existing older-platform message naming `GET /packages/{id}/build/status`. A 5xx answer or a transport error SHALL be retried on the next poll, and the third in a row SHALL end the deploy with that error. Any other error SHALL end the deploy at once.

#### Scenario: the platform no longer knows the package
- **WHEN** the status route answers 404 with the error envelope and code 2006
- **THEN** the deploy exits 1, its error shows HTTP status 404 and code 2006, and the instance is not deployed

#### Scenario: an older platform
- **WHEN** the status route answers 404 without the error envelope
- **THEN** the deploy exits 1 with the older-platform message naming `GET /packages/{id}/build/status`

#### Scenario: one server error
- **WHEN** the status route answers 503 once, then `completed`
- **THEN** the deploy succeeds

#### Scenario: repeated server errors
- **WHEN** the status route answers 503 three times in a row
- **THEN** the deploy exits 1 with that error

### Requirement: Timeout and interrupt
`--timeout` and Ctrl-C SHALL end the wait for the build status as they ended the build watch, with the same messages and exit codes, and SHALL close the stream.

#### Scenario: the build outlasts the timeout
- **WHEN** the status route keeps answering `building` past `--timeout`
- **THEN** the deploy exits with the timeout message that names the package and suggests a longer `--timeout`

#### Scenario: Ctrl-C during the build
- **WHEN** the user presses Ctrl-C while the status route answers `building`
- **THEN** the deploy ends with the canceled message and does not deploy the instance
