# application-capabilities Specification

## Purpose
TBD - created by archiving change application-capabilities. Update Purpose after archive.

## Requirements

### Requirement: The manifest carries an application-capabilities block
The manifest SHALL accept a top-level `application-capabilities` mapping and keep its keys and values
exactly as written. An absent or empty block SHALL mean the manifest has none.

#### Scenario: Block read as written
- **WHEN** `vcr.yml` has `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}`
- **THEN** the block the CLI sends is `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`

#### Scenario: Template manifest keeps the block
- **WHEN** `vcr init` lays its answers over a template `vcr.yml` that has an `application-capabilities` block
- **THEN** the written `vcr.yml` still has the block with the same content

### Requirement: The block and the capabilities list are exclusive
`vcr deploy` and `vcr debug` SHALL exit 1, before any platform call, when the manifest has both
`application-capabilities` and `instance.capabilities`, or when `vcr deploy --capabilities` is given with a
block. The error SHALL name both.

#### Scenario: Both in the manifest
- **WHEN** `vcr deploy` runs with a manifest that has both
- **THEN** it exits 1 with an error naming `application-capabilities` and `instance.capabilities`, and makes no request

#### Scenario: Flag and block
- **WHEN** `vcr deploy --capabilities voice` runs with a manifest that has a block
- **THEN** it exits 1 with an error naming `application-capabilities` and `--capabilities`, and makes no request

### Requirement: Only the instance_url token is accepted
`vcr deploy` and `vcr debug` SHALL exit 1, before any platform call, when a string value in the block
contains a `{{...}}` token other than `{{instance_url}}`. The error SHALL list the unknown tokens and the
valid one. A block with no token is valid.

#### Scenario: Unknown token
- **WHEN** an address is `{{instance_host}}/answer`
- **THEN** the command exits 1 with an error listing `{{instance_host}}` and `{{instance_url}}`, and makes no request

#### Scenario: Literal address
- **WHEN** an address is `https://example.com/answer`
- **THEN** the block is sent unchanged

### Requirement: Deploy sends the block
`vcr deploy` SHALL send a manifest's block as the `applicationCapabilities` object of the package-create
request, unchanged and on every API version. Without a block, the request SHALL be byte-identical to the
request without this change.

#### Scenario: Block sent
- **WHEN** `vcr deploy` runs with a block
- **THEN** `POST /packages` carries `applicationCapabilities` equal to the block, and `capabilities` is `{}`

#### Scenario: No block
- **WHEN** `vcr deploy` runs with `instance.capabilities: [voice]` and no block
- **THEN** `POST /packages` carries no `applicationCapabilities` key

### Requirement: Deploy shows the applied webhook addresses
When the deploy response carries an `applicationCapabilities` object, the deploy summary SHALL add one
line per string field named `address` in it, labelled with the field's dotted path, after the instance host
lines, in path order. Without that object, the summary SHALL be unchanged. When the CLI sent a block and the
response has no `applicationCapabilities`, the CLI SHALL print a warning on stderr that the region's
platform did not apply it, and still exit 0.

#### Scenario: Addresses printed
- **WHEN** the response has `applicationCapabilities: {"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}`
- **THEN** the summary has `Webhook voice.webhooks.answer_url: https://h.example/answer` and then `Webhook voice.webhooks.event_url: https://h.example/event`

#### Scenario: Platform without support
- **WHEN** the CLI sent a block and the response has no `applicationCapabilities`
- **THEN** the deploy exits 0, the summary is the usual one, and stderr says the block was not applied

### Requirement: Debug sends the block and guards the production application
`vcr debug` SHALL send the block as `applicationCapabilities` on the debug-session request, print the applied
addresses or the not-applied warning as `vcr deploy` does, and exit 1 before starting the session when a
block is set and the session's application id equals `instance.application-id`. The application id is
the one left after any confirmation prompt. The error SHALL say to set `debug.application-id` or `--app-id`
to a different application.

#### Scenario: Debug falls back to the instance application
- **WHEN** a manifest with a block has no `debug.application-id` and `vcr debug --yes` runs
- **THEN** it exits 1 with the guard error and makes no debug-session request

#### Scenario: Separate debug application
- **WHEN** a manifest with a block has a `debug.application-id` different from `instance.application-id`
- **THEN** the debug-session request carries `applicationCapabilities` equal to the block

#### Scenario: No block
- **WHEN** a manifest without a block has no `debug.application-id`
- **THEN** debug falls back to the instance application id as it does today
