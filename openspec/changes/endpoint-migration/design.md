# Design

## Context

- `internal/endpoint/bootstrap.go` picks the platform API base URL from `endpoint.Inputs` (`Endpoint`, `GraphQLFlag`, `Region`, `GraphQLConfig`) and maps a GraphQL URL with `mapGraphQL`: `<base>/api/graphql/v1/graphql` becomes `<base>/api/deployment`, a production `graphql.<euw1|use1|apse1|apse2>.runtime.vonage.cloud/v1/graphql` URL becomes `ProductionDefault`.
- `internal/cli/prerun.go` `preRun` runs `loadConfig` then `startConnecting` then `initPlatform` for the `full` and `config-only` platform modes. `none` commands (`configure`, `upgrade`, help, completion, `--version`) stop before reading the config file.
- `internal/config` reads and writes `~/.vcr-cli` with `gopkg.in/ini.v1`. `layout.go` lists the four keys; `write.go` merges into the existing document so unknown keys, sections and comments survive, and writes through a temp file at 0600.
- `internal/term` decides the output mode once (`Decide`): rich, static or classic. `IO.CanPrompt()` is true when stdin and stdout are terminals. `Prompter.YesNo` asks `<question> [y/n]: ` on stdout and returns false at EOF.
- `vcr configure` writes `graphql_endpoint` from `graphqlEndpointToWrite` (`internal/cli/configure/regions.go`) and builds its own resolver in `newResolver` (`internal/cli/configure/configure.go`).
- The contract suite replays classic runs against v2.6.1 transcripts. Its default config (`test/contract/sandbox.go` `defaultConfig`) sets only `graphql_endpoint`.

## Goals / Non-Goals

**Goals:**
- A supported, documented name for the platform URL: `--endpoint` and the `endpoint` config key.
- Users on GraphQL settings find out, on every run, in a way that never blocks a script.
- A file migrated by v3 still works with v2.

**Non-Goals:**
- Removing the GraphQL settings or their mapping.
- Remembering a declined prompt.

## Decisions

### Precedence

`endpoint.Inputs` gains `ConfigEndpoint`. `Bootstrap` order: `Endpoint` flag, `GraphQLFlag`, `Region` flag (production default), `ConfigEndpoint` (as-is), `GraphQLConfig` (mapped), production default. Flags beat the file, as they do today; the `--region` flag keeps beating the file's own URL, as it does today for `graphql_endpoint`. The config value is used as-is, like the flag: the resolver already refuses anything that is not https or loopback http.

`Bootstrap` also reports where the URL came from (`Source`: flag, graphql flag, region, config endpoint, config graphql, default) and, for the two GraphQL sources, the mapped URL and whether it was the production form. The pre-run uses that to choose the notice; nothing else needs it.

Rejected: a separate function that re-derives the source from the inputs — it would repeat the precedence and could drift from it.

### Where the migration runs

In `preRun`, after `loadConfig` and before `startConnecting`, for the `full` and `config-only` modes only. Before `startConnecting` so the question is never drawn under the rich "connecting" step. `none` commands never read the file, so they never migrate.

Rejected: in `initPlatform` — it runs after the connecting step has started.
Rejected: in `vcr configure` only — the user asked for a prompt on normal commands.

### Ask or tell

Ask when `env.IO.Mode() != term.Classic && env.IO.CanPrompt()`; tell otherwise. Classic covers pipes, CI (`CI` set), `TERM=dumb` and `VCR_UI=classic`; `CanPrompt` covers stdin redirected from a file in an otherwise interactive terminal. The flag notice is always a notice: a flag is in a command line someone wrote down, so there is no file to fix.

### Messages

Notices go to stderr with the warning icon, one line each, so stdout parsers and exit codes are untouched. Exact text (`<path>` is the file as the user named it, `<url>` the mapped URL):

- Flag, gateway form: `! --graphql-endpoint is deprecated; use --endpoint <url> instead`
- Flag, production form: `! --graphql-endpoint is deprecated and not needed; choose the region with --region`
- File, told: `! graphql_endpoint in <path> is deprecated; add "endpoint = <url>" to the file, or run vcr configure`
- File, asked (stdout, through `YesNo`): `graphql_endpoint in <path> is deprecated. Add "endpoint = <url>" to the file?`
- After `y`: `✓ Added endpoint to <path>` on stdout
- Write failure: `! Could not update <path>: <error>` on stderr

These wordings are the planner's call; the reviewer of the change may adjust them.

### Writing the key

A new `config.AddEndpoint(path, url string) error` loads the document, sets only the top-level `endpoint` key and writes it with the existing private temp-file-and-rename path (`replaceFile`, in-place fallback). It does not go through `File`: rewriting all four values from what was parsed could change how a secret is quoted.

`File` gains `Endpoint`; `fileFrom` reads it and `applyTo` writes it only when it is non-empty, so a zero `File` keeps the v2.6.1 layout. In a new file it comes after `default_region`.

Rejected: removing `graphql_endpoint` during migration — v2 would silently fall back to production.

### Configure

`fileToWrite` sets `Endpoint` to the bootstrap URL `newResolver` used, so `endpoint` and `graphql_endpoint` always describe the same platform.

### Help

`--endpoint` is no longer hidden; its usage stays `Platform API base URL (overrides --graphql-endpoint and the config file)`. `--graphql-endpoint`'s usage becomes `Deprecated: use --endpoint. GraphQL endpoint used to fetch metadata`. It is not marked with cobra's `MarkDeprecated`, which hides the flag and prints its own message on every use.

### Contract suite

`defaultConfig` gains `endpoint = ${FAKE}/v1/vcr/euw1/api/deployment` so scenarios that don't test the migration see no notice. v2.6.1 ignores the key, so the transcripts stay valid. Scenarios whose output now differs on purpose get `allowed-diffs.txt` lines: help output (the new flag line), `--graphql-endpoint` scenarios (stderr notice), scenarios with their own `config-file` holding only `graphql_endpoint` (stderr notice), and `configure-*` (the written file).

## Risks / Trade-offs

- A declined question repeats on every interactive run. Accepted for now; remembering it needs another key (Out of scope).
- Two keys can disagree if edited by hand. `endpoint` wins in v3, `graphql_endpoint` in v2, which matches what each version understands.
- New stderr lines can trip a pipeline that fails on any stderr output. The notice names exactly what to change to make it go away.
- Help for every command grows one line, so the docs pages and help scenarios change.
