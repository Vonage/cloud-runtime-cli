Branch: `feature/debug-forward-platform-calls`

## Why

The newer platform SDKs authenticate with a platform token that only a workload running on the platform can obtain. Under `vcr debug` the app runs locally, so those SDKs can neither reach their services nor get a token. The platform's debug server can make the call with its own identity if the CLI tells it which service and path to call, instead of a URL.

## What Changes

- The local app's environment gains `VCR_DEBUG_PROXY_URL=http://127.0.0.1:<debugger-port>`, so an SDK can find the local debugger without assuming the default port.
- A request to the local debugger at `/_vcr/<service>/<rest>` is relayed as an `execute-remote` with `request.broker_auth: true`, `request.faas_function: <service>`, `request.path: /<rest>` (percent-encoding kept) and no URL. The query, method, headers and body are relayed as today.
- A `/_vcr/` request whose service is not a DNS label, or that has no service segment, is answered 400 locally and never relayed.
- Every other request is relayed exactly as before.

## Capabilities

### New Capabilities

- `debug-platform-calls`: how `vcr debug` relays a local app's calls to platform services that authenticate with a platform token.

### Modified Capabilities

None.

## Impact

- `internal/debugsession/env.go`: the new variable.
- `internal/debugsession/handler.go` and `protocol.go`: the `/_vcr/` mapping and two new `execute-remote` request fields.
- `test/fakeplatform`: the peer records the new fields.
- No change to flags, config files or the existing relay.

The SDKs send their calls to `/_vcr/<service>/...` when `VCR_DEBUG_PROXY_URL` is set.

## Rollout

Ships after the platform's debug server understands `broker_auth` in every production region, not just after it is released. Before that, a relayed `/_vcr/` call fails at the debug server, which only affects SDKs that use the new path. Released through the normal CLI release. Nothing changes for apps that don't use the newer SDKs.

## Out of scope

- Choosing the target host or token in the CLI: the platform's debug server decides both, so a local process can't point a platform token at another host.
- Debug data isolation: a debug session has its own instance identity, so instance-scoped data is separate from the deployed instance's, as it already is for the existing state provider. Documented, not changed.
