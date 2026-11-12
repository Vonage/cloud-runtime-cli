# Proposal

Branch: `feature/application-capabilities`, after the `application-capabilities` change in the same pull request.

## Why

The `application-capabilities` block in `vcr.yml` is the Vonage Application API `capabilities` object, written by
hand. Nothing in the CLI or an editor tells a user which keys exist or what a working block looks like, so the first
mistake shows up as a platform error at deploy time.

## What Changes

- New command `vcr app capabilities add` writes a default block for each capability flag into the manifest:
  `--voice`, `--messages`, `--rtc`, `--verify`, `--video`, `--vbc`, `--meetings` and `--network-apis` (with
  `--network-application-id` and `--redirect-uri`). Every webhook address is `{{instance_url}}/<capability>/<event>`.
- A capability already in the block is skipped; `--force` replaces it. `instance.capabilities` is removed when the
  block is written, because the two cannot be set together. `--print` writes the YAML to stdout and leaves the file
  alone.
- The edit keeps the rest of the file as written: comments, key order and keys the CLI does not know.
- New file `schema/vcr.schema.json`: a JSON Schema for the whole manifest. Its `application-capabilities` part is
  generated from the Vonage Application API v2 OpenAPI definition (v2.4.1) and committed; `make schema SPEC=<file>`
  regenerates it. Editors that use yaml-language-server get completion and checks from it.

## Capabilities

### New Capabilities

- `capabilities-generator`: the `vcr app capabilities add` command, its defaults and how it edits the manifest.
- `manifest-schema`: the published JSON Schema for `vcr.yml` and what it accepts.

### Modified Capabilities

None. `vcr deploy` and `vcr debug` read the block as they do today.

## Impact

- New: `internal/cli/app/capabilities*.go`, `internal/manifest/appcaps_defaults.go`, a YAML node editor in
  `internal/manifest`, `schema/vcr.schema.json`, a generator under `tools/schemagen`, a `schema` Makefile target.
- `vcr app` gains a child, so its help page, the contract command tree, the help goldens and the contract scenarios
  change.
- Test-only dependency: a JSON Schema validator for the schema test.
- Builds on the `application-capabilities` change, which adds the block this command writes; both ship in one pull
  request.

## Rollout

Ships in the same release as the `application-capabilities` change. Once that release is out, register the
schema with SchemaStore in a separate pull request to `SchemaStore/schemastore`, matching every manifest file name
the CLI reads.

## Out of scope

- The SchemaStore registration: it needs the schema URL to exist, so it follows the release.
- `vcr init` templates writing the block: changes every new project's manifest.
- `vcr app create` writing the block: it creates an application, not a manifest.
- Checking the block against the schema during `vcr deploy`: the payload stays Vonage's contract, as today.
- A `--path-prefix` flag: the defaults are meant to be edited by hand.
- `fallback_answer_url` and the other video events in the defaults: written by hand when needed.
