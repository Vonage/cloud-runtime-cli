# Design

## Context

Today `deploy.API.WatchBuild` (`internal/platform/watch.go`) both streams build frames and decides the outcome: `frameStatus` matches the server's exact `Status - completed` / `Status - failed` line, and a normal close before that line is an error. Three security scenarios (`security-build-frame-*`) exist to prove that a log line cannot imitate the status line.

The deployment API is adding `GET /packages/:packageID/build/status` on `v0.3`, `v0.4` and `v0.5`; it runs in the development environment but has not shipped to every region yet (see the rollout gate in the proposal). It answers `200 {"packageId","status"}` with `pending`, `building`, `completed` or `failed`, read from the package row, without waiting. Unknown, deleted, other-account and non-UUID ids answer 404 code 2006; a store failure answers 500 code 1001. A Job informer writes the row, with a 5-minute sweep for rows whose Job is gone. The server's watch sends its own status line and then closes with a normal close, before the row is written, so the stream usually ends a moment before the route says `completed`.

## Goals / Non-Goals

**Goals:**
- The status route alone decides the build outcome, in classic and rich output.
- The stream is best effort: its failures cost output, not the deploy.

**Non-Goals:**
- Changing the stream's connect retries, reconnects or frame limit.
- Changing any message other than those the spec names.

## Decisions

1. **Poll and stream run together, and the stream is always joined.** The build stage starts the watch in a goroutine under a child context and polls in its own goroutine. When the poll is final, the stage waits for the watch goroutine to return or for the 5-second grace to pass, whichever is first. The grace is timed with `clk.Sleep` in a goroutine of its own under a cancellable context, because `clock.Clock` has only `Sleep`. If the grace passes, the stage cancels the child context, which closes the stream, and then waits for the watch goroutine to return. Only then does it print the package-built line or return. Rejected: polling only after the stream ends (option B), because a stream that hangs would still hold the deploy until `--timeout`.
2. **`platform.Client.BuildStatus(ctx, packageID) (string, error)`.** One GET through `core.read` with a new `routeBuildStatus = readRoute{label: "GET /packages/{id}/build/status"}`. `notFound` stays false: a 404 with the envelope is an ordinary API error showing its status and code, and a 404 without one gets the older-platform message, as for the other reads. The package id goes through `checkServerID` first, as `WatchBuild` does. An answer outside the four statuses is an error naming the value.
3. **The poll loop lives in `internal/deploy`** (`awaitBuild`), with `clock.Clock` for the 2-second interval so tests run on `clocktest.Fake`. The interval and the 5-second grace are constants. `deploy.Plan` does not change. `deploy.Run` gains a `clock.Clock` parameter, which `internal/cli/deploy` fills from `cmdenv.Env.Clock`.
4. **Retries.** A `*clierr.APIError` with a 5xx `HTTPStatus`, or a transport error, is retried on the next tick; three in a row end the deploy with the last error. A transport error is a `*url.Error` that is not a `*policyError`, the same test `retryReason` uses in `internal/platform/upload_retry.go`; the platform package exports a small `IsTransportError(err) bool` for this. Context errors, decode errors, unknown statuses and id refusals are never retried. Rejected: retrying 404s, because the package was created a moment earlier, so a 404 means the platform lost it or the route is missing.
5. **`WatchBuild` keeps its signature but only streams.** It no longer inspects frames: `frameStatus`, `statusPrefix` and the status constants are deleted. A normal close returns `nil`, and the build stage says nothing about it. A connect failure or the read-limit error returns that error, which the build stage turns into the single warning line. Reconnects after a drop are unchanged. A context end returns the context error unwrapped, and the build stage ignores it when the stage caused it.
6. **Messages are unchanged** for success, failure, timeout and cancel: the build stage keeps returning `failed to watch deployment for package_id=%q: %w`. `endedError` moves from `watch.go` to the build stage with identical texts. When the poll's context has a passed deadline but `ctx.Err()` is still nil, the stage passes `context.DeadlineExceeded`, the rule `ctxEnded` in `internal/platform/websocket.go` uses, so the message never wraps `nil`.
7. **The contract harness counts the status route as a read.** `isReadRoute` in `test/contract/reads.go` accepts `/packages/<id>/build/status`, so the classic transcripts (recorded from a release that never calls it) still compare, and every scenario that reaches the build lists the status read in `expect-reads`. The fake serves the route from `readRoutes` with a new `buildStatus` fixture: a list of answers (status or HTTP code), the last repeating. The default answers `completed`.

Calls marked as mine, not agreed in conversation: the 2-second interval, the 5-second grace, three consecutive server errors, and the wording of the stderr warning.

## Risks / Trade-offs

- **Status lags the stream:** the stream usually ends before the row says `completed`. The poll keeps going, so the cost is up to one interval. The grace period covers the opposite case.
- **Build retries look odd:** the build Job retries a failed pod up to twice, and the server's watch reports on the first pod to finish. A first pod that fails sends `Status - failed` and closes the stream while the row stays `building`. The CLI then waits silently through the retry, and may report success after a `Status - failed` line. That is the intended outcome, but the output can confuse a user.
- **A build whose Job vanishes** settles only when the server's 5-minute sweep runs. The deploy waits for it, within `--timeout` (default 10 minutes).
- **Older regions:** every deploy fails in a region without the route. The rollout gate in the proposal blocks the release until every region has it.
- **Timing in the contract harness:** `expect-reads` counts every status GET, and the binary polls on the wall clock from package creation. The timeout and interrupt scenarios are set up so exactly one status read lands before they end (tasks 5.2).
- **More requests:** one GET every 2 seconds per deploy, which is small next to the upload.
