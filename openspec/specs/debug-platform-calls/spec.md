# debug-platform-calls Specification

## Purpose
How `vcr debug` lets a locally running app reach a platform service that needs a platform token: the app is told the local debugger's address, and calls to `/_vcr/<service>/...` are relayed to the debug server by service name and path.

## Requirements

### Requirement: The local app is told where the debugger is

`vcr debug` SHALL start the local app with `VCR_DEBUG_PROXY_URL` set to `http://127.0.0.1:<debugger-port>`, where the port is the one the local debugger listens on.

#### Scenario: Default port

- **WHEN** `vcr debug` runs without `--debugger-port`
- **THEN** the app's environment has `VCR_DEBUG_PROXY_URL=http://127.0.0.1:3001`

#### Scenario: A chosen port

- **WHEN** `vcr debug --debugger-port 4100` runs
- **THEN** the app's environment has `VCR_DEBUG_PROXY_URL=http://127.0.0.1:4100`

### Requirement: Platform service calls are relayed by service and path

A request to the local debugger whose path is `/_vcr/<service>` or starts with `/_vcr/<service>/` SHALL be relayed as an `execute-remote` whose `request` has `broker_auth` true, `faas_function` `<service>`, `path` the remainder of the path as the app sent it, percent-encoding kept (`/` when there is none), and an empty `url`, with the request's method, query, headers and body relayed as for any other call. The answer SHALL be replayed to the app as for any other call.

#### Scenario: A data call

- **WHEN** the app sends `PUT /_vcr/data-api/v1/items?collection=orders` with a JSON body
- **THEN** the debug server receives an `execute-remote` with `broker_auth` true, `faas_function` `data-api`, `path` `/v1/items`, `url` empty, method `PUT`, query `collection=orders` and the same body
- **AND** the app receives the status, headers and body of the `execute-remote` answer

#### Scenario: An encoded path segment

- **WHEN** the app sends `GET /_vcr/data-api/v1/collections/a%2Fb`
- **THEN** the `execute-remote` has `path` `/v1/collections/a%2Fb`

### Requirement: Malformed platform service calls are refused locally

A request whose path starts with `/_vcr/` but whose service segment is empty or not a DNS label (lowercase letters, digits and hyphens, 1 to 63 characters, not starting or ending with a hyphen) SHALL be answered 400 by the local debugger and SHALL NOT be relayed.

#### Scenario: A host in the service segment

- **WHEN** the app sends `GET /_vcr/data-api.example.com/v1/items`
- **THEN** the app receives 400 and nothing is sent through the tunnel

### Requirement: Other calls are relayed as before

A request whose path does not start with `/_vcr/` SHALL be relayed exactly as before this change, with no `broker_auth` and no `path`.

#### Scenario: An existing provider call

- **WHEN** the app sends `POST /?func=example-provider`
- **THEN** the `execute-remote` has `faas_function` `example-provider`, `url` `http://example-provider/` and no `broker_auth`
