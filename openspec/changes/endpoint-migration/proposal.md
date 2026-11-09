# Proposal

## Why

The CLI no longer talks GraphQL, but the only way to point it at a platform is still a GraphQL URL: the `graphql_endpoint` config key or the `--graphql-endpoint` flag, which the CLI maps to the platform API. Users have no supported name for the URL the CLI really uses, and no warning that the GraphQL settings will go away. This change adds that name and moves people onto it: a question on an interactive terminal, a one-line notice everywhere else, including CI/CD pipelines that pass `--graphql-endpoint`.

## What Changes

- A new `endpoint` key in the config file names the platform API base URL. It wins over `graphql_endpoint`.
- The `--endpoint` flag is no longer hidden, and `--graphql-endpoint` is described as deprecated in help.
- When the platform URL comes from `--graphql-endpoint`, the CLI prints a one-line notice on stderr naming the replacement: `--endpoint <url>` for a gateway URL, `--region aws.<name>` for a production GraphQL URL. Never a question.
- When it comes from `graphql_endpoint` in the config file:
  - on an interactive terminal (rich or static output), the CLI asks whether to add `endpoint = <url>` to the file, writes it on "y", and carries on;
  - otherwise (classic output: CI, pipes, `VCR_UI=classic`), it prints a one-line notice on stderr saying what to add.
- Migrating adds `endpoint` and keeps `graphql_endpoint`, so an older CLI reading the same file still works.
- `vcr configure` writes `endpoint` as well as `graphql_endpoint`.
- Exit codes and stdout of every command are unchanged.

## Capabilities

### New Capabilities

- `platform-endpoint`: how the CLI chooses the platform API base URL from flags and the config file, and how it moves users off the GraphQL settings.

### Modified Capabilities

None. The baseline `graphql-endpoint` spec describes the GraphQL client the rewrite replaces; this change leaves it as it is.

## Impact

- Code: the config file package, endpoint bootstrap, the root pre-run, `vcr configure`, the global flags.
- Output: a new stderr line for users still on GraphQL settings; one new line in every command's help.
- Tests: the contract suite's default config gains an `endpoint` key so unrelated scenarios stay identical; the scenarios that exercise the GraphQL settings, `configure` and help get entries in `allowed-diffs.txt`.

## Branch

`feature/cli-rewrite`: these commits join the open rewrite pull request (see Rollout).

## Rollout

- This change builds on the rewrite in the open `feat!: rewrite the CLI on the deployment API` pull request, not on `main`. Either merge it into that pull request so it ships in v3.0.0, or open it after that pull request merges.
- No server change. A file with both keys works with v2 and v3 CLIs.
- Removing `graphql_endpoint` and `--graphql-endpoint` is a later change, once the GraphQL service is switched off.

## Out of scope

- Remembering a "no" at the prompt: the question repeats on each interactive run until the file has `endpoint`; a stored "don't ask again" setting is a new config key, left until users ask for it.
- Removing `graphql_endpoint` from the file during migration: an older CLI would then fall back to production.
- An environment variable for the endpoint: not requested.
- Changing the baseline `graphql-endpoint` spec: it belongs to the code the rewrite already replaced.
