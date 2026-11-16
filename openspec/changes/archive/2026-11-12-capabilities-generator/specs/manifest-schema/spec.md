# Spec Delta

## Purpose
Defines the JSON Schema for `vcr.yml` that the repository publishes for editors, what it covers and what it
accepts.

## ADDED Requirements

### Requirement: A published schema for the manifest
The repository SHALL carry `schema/vcr.schema.json`, a JSON Schema (draft-07) for the manifest. It SHALL describe
`project`, `instance`, `debug` and `application-capabilities` with the keys and value types the CLI reads, and SHALL
be reachable at a stable URL on the default branch.

#### Scenario: A manifest the CLI accepts passes
- **WHEN** a manifest the CLI deploys without error is checked against the schema, and its string fields hold
  strings, its integer fields integers and its boolean fields true or false
- **THEN** it passes

#### Scenario: An empty key passes
- **WHEN** `debug:` has no value
- **THEN** it passes

#### Scenario: A wrong type fails
- **WHEN** a manifest has `instance.scaling.min-scale: "two"`
- **THEN** it fails the schema check at `instance.scaling.min-scale`

### Requirement: The capabilities part follows the Vonage Application API
The `application-capabilities` part of the schema SHALL match the `capabilities` object of the Vonage Application
API v2 definition, version 2.4.1: `voice`, `messages`, `rtc`, `verify`, `video`, `vbc`, `meetings` and
`network_apis`, with their fields, types and enums. Addresses SHALL allow the `{{instance_url}}` token.

#### Scenario: Every generated default passes
- **WHEN** the output of `vcr app capabilities add` with every capability flag is checked against the schema
- **THEN** it passes

#### Scenario: A wrong enum fails
- **WHEN** `application-capabilities.voice.region` is `mars-1`
- **THEN** it fails the schema check at `application-capabilities.voice.region`

### Requirement: Unknown keys are allowed
The schema SHALL accept keys it does not describe, at every level, so a field added to the Vonage API later loses
only completion.

#### Scenario: A new Vonage field
- **WHEN** `application-capabilities.voice` has a key `new_field: 1` that the schema does not describe
- **THEN** it passes the schema check
