# Spec Delta

## MODIFIED Requirements

### Requirement: Deploy shows the applied webhook addresses
When the deploy response carries an `applicationCapabilities` object, the classic deploy summary SHALL add one
line per string field named `address` in it, labelled with the field's dotted path, after the instance host
lines, in path order. On a terminal the addresses SHALL appear in the application webhooks box that the
`deploy-progress` capability describes. Without that object, the summary SHALL be unchanged. When the CLI sent a
block and the response has no `applicationCapabilities`, the CLI SHALL print a warning on stderr that the region's
platform did not apply it, and still exit 0.

#### Scenario: Addresses printed
- **WHEN** the response has `applicationCapabilities: {"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}`
- **THEN** the classic summary has `Webhook voice.webhooks.answer_url: https://h.example/answer` and then `Webhook voice.webhooks.event_url: https://h.example/event`

#### Scenario: Platform without support
- **WHEN** the CLI sent a block and the response has no `applicationCapabilities`
- **THEN** the deploy exits 0, the summary is the usual one, and stderr says the block was not applied
