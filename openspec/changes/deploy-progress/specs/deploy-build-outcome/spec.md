# Spec Delta

## MODIFIED Requirements

### Requirement: The build log stream only shows output
The CLI SHALL use the build-watch stream's frames only as build output, and SHALL NOT end or fail the build because
of any frame's content. In the classic presentation the CLI SHALL show every frame as it arrives; on a terminal it
SHALL show them as the `deploy-progress` capability describes. When the server closes the stream normally, the CLI
SHALL say nothing and keep waiting on the status route.

#### Scenario: the stream closes before the status is final
- **WHEN** the stream sends one line and closes normally, and the status route then answers `completed`
- **THEN** the deploy succeeds, in the classic presentation the line is shown before the package-built line, and
  stderr is empty
