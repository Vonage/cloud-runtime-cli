## 1. Environment

- [x] 1.1 Test first in `internal/debugsession/env_test.go`: `debugEnvironment` with a `DebuggerPort` of 3001 contains `VCR_DEBUG_PROXY_URL=http://127.0.0.1:3001`, and with 4100 contains `...:4100`. Add `DebuggerPort` to `debugVars` in `internal/debugsession/env.go`, the variable to the fixed list, and pass `s.cfg.DebuggerPort` in `internal/debugsession/run.go`. Verify: `go test ./internal/debugsession/ -run 'TestDebugEnvironment'` (`TestDebugEnvironmentNamesAndValues` and `TestDebugEnvironmentHasNoUnlistedNames` list the variables and gain the new one). The contract test app prints every variable name, so each debug scenario whose stdout gains `testapp env VCR_DEBUG_PROXY_URL` gets its own `stdout` line in `test/contract/allowed-diffs.txt`, and the `debug-on-terminal` rich golden is re-recorded with `make contract-rich-record`.

## 2. Wire fields

- [x] 2.1 Test first in `internal/debugsession/protocol_test.go`: a `remoteRequestMessage` with zero `BrokerAuth` and `Path` marshals with no `broker_auth` or `path` key, and one with them set marshals `"broker_auth":true,"path":"/v1/items"`. Add both fields to `remoteCommand` in `protocol.go` with `omitempty`, and to `remoteRequest` in `handler.go`; copy them in `tunnel.go`'s `call`. Verify: `go test ./internal/debugsession/ -run TestWireMessages`

## 3. The `/_vcr/` mapping

- [x] 3.1 Test first in `internal/debugsession/handler_test.go`: `PUT /_vcr/data-api/v1/items?collection=orders` with a body reaches the fake caller as `BrokerAuth` true, `FAASFunction` `data-api`, `Path` `/v1/items`, `URL` empty, method, query and body unchanged; `GET /_vcr/data-api` gives `Path` `/`; `GET /_vcr/data-api/v1/collections/a%2Fb` gives `Path` `/v1/collections/a%2Fb` (scenario "An encoded path segment"). Implement the prefix branch in `newRemoteRequest`, reading `r.URL.EscapedPath()`. Verify: `go test ./internal/debugsession/ -run TestHandler`
- [x] 3.2 Test first in `handler_test.go`: `/_vcr/`, `/_vcr//x`, `/_vcr/Data-API/x`, `/_vcr/-x/y` and `/_vcr/data-api.example.com/x` answer 400 and the fake caller records no call. Verify: `go test ./internal/debugsession/ -run TestHandler`. The `/_vcr/` tests of 3.1 and 3.2 live in `internal/debugsession/handler_service_test.go`, because `handler_test.go` would pass the 400-line limit.
- [x] 3.3 Confirm `TestHandlerProviderAndPathPrecedence` and the other existing handler tests pass unchanged (scenario "An existing provider call"). Verify: `go test ./internal/debugsession/ -run TestHandler`

## 4. Through the tunnel

- [x] 4.1 Test first in `internal/debugsession/tunnel_calls_test.go`, against `test/fakeplatform`: a `/_vcr/data-api/v1/items` call arrives at the peer with `broker_auth` and `path`, and the peer's answer is replayed to the app. Extend `test/fakeplatform/peertypes.go`'s remote request type with the two fields. Verify: `go test ./internal/debugsession/ ./test/fakeplatform/...`. The test starts the shared fake platform (`fakeplatform.Start`) rather than the in-package local peer that the rest of `tunnel_calls_test.go` uses, and sends the call through `newLocalHandler`.

## 5. Docs and checks

- [x] 5.1 Add `VCR_DEBUG_PROXY_URL` and the `/_vcr/<service>/` path to the ENVIRONMENT VARIABLES section of `internal/cli/debug/text.go`. Verify: `go run . debug --help` shows both. The help page changes `test/contract/testdata/golden-help/vcr_debug.txt`, the `vcr debug` entry of `test/contract/testdata/contract-tree.json` and `docs/vcr_debug.md` (`make docs`), and `help-debug` gets a `stdout` line in `test/contract/allowed-diffs.txt`; no make target writes the first two, so they were edited to match `go run . debug --help`.
- [ ] 5.2 Run the repo's checks. Verify: `make test && golangci-lint run`
