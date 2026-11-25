## Context

The local debugger (`internal/debugsession/handler.go`) turns every request from the app into an `execute-remote`: the provider and target path come from query parameters or the request path, and the URL is `http://<provider><path>`. The platform's debug server then calls that URL with a token it signs. The newer SDKs need a different token and an HTTPS host the debug server picks, so the CLI must send the service and path and leave the URL to the server.

## Goals / Non-Goals

**Goals:** a path-based form the newer SDKs can use as a plain base URL; legacy messages unchanged byte for byte.

**Non-Goals:** any knowledge in the CLI of platform hosts, schemes or tokens.

## Decisions

### A `/_vcr/<service>` path prefix

An SDK sets its base URL to `$VCR_DEBUG_PROXY_URL/_vcr/<service>` and appends its own paths, so it needs no per-request query parameter. `/_vcr/` can't collide with legacy calls, which put the provider in the query.

- Rejected: a query parameter like the legacy form. The SDK's request builder owns the query, and adding one to every call is more change in the SDK for the same result.
- Rejected: a separate local port for new services. A second listener means a second flag, a second set of loopback checks and a second variable.

### Two optional request fields

`remoteCommand` gains `BrokerAuth bool \`json:"broker_auth,omitempty"\`` and `Path string \`json:"path,omitempty"\``. With `omitempty` a legacy message serialises exactly as before.

- Rejected: a new operation. The pending-call table and the answer path key on `execute-remote` already.

### The path is taken escaped

The prefix branch reads `r.URL.EscapedPath()`, not `r.URL.Path`, so a `%2F` inside a segment reaches the service as `%2F` instead of becoming a path separator. The legacy branch keeps reading `r.URL.Path`, unchanged.

### Validate the service locally

The CLI checks the service segment against the DNS-label rule and answers 400 before relaying. The debug server checks it too; checking here gives the developer a local, immediate error instead of a round trip.

### `127.0.0.1`, not `localhost`

The listener always binds `127.0.0.1` and binds `[::1]` only when IPv6 is available (`listen.go`). `localhost` can resolve to `::1` first on some systems, so the variable names the address that always answers.

## Risks / Trade-offs

- [A CLI released before the debug server understands `broker_auth`] → the server treats the call as legacy with an empty URL and builds `http://<service>/`, which fails. Only apps using the newer SDKs are affected, and the rollout order (server first) avoids it.
