# Proposal

Branch: `feature/build-status-polling`, cut from the rewritten CLI, not from `main`. This change builds on the rewrite and ships with it or after it.

## Why

`vcr deploy` decides that a build has finished by matching the exact text `Status - completed` or `Status - failed` in the build-watch WebSocket stream. The outcome therefore depends on a log stream: a stream that closes early, drops or never connects fails the deploy even when the build succeeded, and the CLI needs special rules so that a build log line cannot imitate the status line. The platform now serves `GET /<version>/packages/<id>/build/status`, which answers the stored build state (`pending`, `building`, `completed` or `failed`). The CLI should take the outcome from that route and use the stream only to show logs.

## What Changes

- `vcr deploy` polls the build status route from the moment the package is created until it answers `completed` or `failed`. That answer alone decides whether the deploy continues or fails.
- The build-watch stream only shows build output. No frame decides the outcome. When the status becomes final, the CLI waits a short grace period for the stream to end on its own so the last lines are shown, then closes it.
- A stream that cannot connect, closes early or drops no longer fails the deploy. The deploy keeps waiting on the status route; the build output shown may be incomplete. A stream that cannot connect gets one warning line on stderr; a normal close gets none.
- `--timeout` and Ctrl-C end the wait on the status route as they end the watch today, with the same messages.
- A region whose platform does not serve the status route gets the existing "platform API is older than this CLI" error for that route.
- The test fake serves the status route; the contract scenarios for builds are updated.

Intended differences from earlier releases, each recorded in `test/contract/allowed-diffs.txt`: a deploy whose stream closes early or cannot connect now succeeds when the build succeeded, and a log line that reads `Status - failed` no longer fails a build that succeeded.

## Capabilities

### New Capabilities
- `deploy-build-outcome`: how `vcr deploy` decides that a package build has finished and whether it succeeded.

### Modified Capabilities
None.

## Impact

- `internal/platform`: a new `BuildStatus` call; the build watch loses its status-line matching.
- `internal/deploy`: the build stage runs the status poll and the watch together, in classic and rich output.
- `test/fakeplatform`: the status route and a fixture for the statuses it answers.
- `test/contract`: the status route counts as a read; the build scenarios, their expected reads and the allowed diffs are updated.
- Server dependency: the build status route on every API version (`v0.3`, `v0.4`, `v0.5`) in every region the CLI supports.

## Rollout

1. Merge with or after the rewrite.
2. Before release, the build status route must be live in every production region. The CLI must not be released before that: a missing route fails every deploy in that region.
3. Check by hand against a development environment: a deploy that builds, a deploy whose build fails, and a deploy interrupted with Ctrl-C.
4. Release as part of the rewrite's release.

## Out of scope

- Streaming changes on the server, such as closing the watch after the status line: the grace period covers a server that keeps the stream open.
- A `vcr` command that shows the build status of an existing package: nothing asked for it.
- Falling back to stream matching when the status route is missing: the release waits for the route instead, which keeps one code path.
- Polling backoff or a configurable interval: a fixed interval is enough for builds that take minutes.
