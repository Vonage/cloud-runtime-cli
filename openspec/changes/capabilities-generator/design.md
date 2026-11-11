# Design

## Context

- `manifest.Read` parses `vcr.yml` into a struct with a non-strict decode, and `manifest.Marshal` writes the struct
  back. A round trip through them drops comments, key order and unknown keys, so it cannot be used to edit a
  user's file.
- `manifest.Find` picks the manifest the same way for every command. `manifest.CheckApplicationCapabilities`
  already rejects a block next to `instance.capabilities` or `--capabilities`.
- The contract harness pins the command tree (`test/contract/testdata/contract-tree.json`), the help pages
  (`testdata/golden-help/`, 27 of them) and every command and flag to a scenario (`coverage_test.go`). Classic
  transcripts must be recorded from the baseline release (`make contract-baseline-check`); rich goldens are
  recorded from the binary under test.
- The Vonage Application API v2 definition (OpenAPI 3.0, version 2.4.1) describes `capabilities` twice. The create
  request has seven capabilities; the application response schema also has `meetings`. Both use local `$ref`s
  (`VoiceRegion`, `VideoResponse`, `active_webhook`) and OpenAPI-only keys (`example`,
  `x-nexmo-developer-*`), and some booleans have string defaults (`default: "true"`).

## Goals / Non-Goals

**Goals:**
- One command that writes a working default block for any of the eight capabilities.
- An edit that leaves the rest of the user's file as written.
- A schema that editors can use, kept in step with the defaults by a test.

**Non-Goals:**
- Checking the block during `vcr deploy`, or fetching the Vonage definition at run time.
- Covering every optional field in the defaults.

## Decisions

### `vcr app capabilities add` is a `cmdenv.None` command
It reads and writes a local file only, so it needs no config, credentials or region. `vcr app capabilities` is a
group with `add` as its only child, leaving room for `remove` later without a rename.
- Rejected: a top-level `vcr capabilities`: capabilities belong to the Vonage application, which `vcr app` already
  manages.
- Rejected: flags on `vcr init`: `init` creates a project; adding capabilities to an existing one is the common case.

### The defaults are one table in `internal/manifest/appcaps_defaults.go`
Each entry is a capability name and a function that returns its default as a `map[string]any`; `network_apis`
takes the two flag values. The address rule is `{{instance_url}}/<capability>/<webhook>` with the `_url` suffix
dropped and `_` turned into `-`.
- Rejected: defaults read from the schema file: the schema says what is allowed, not which webhooks to turn on.

### The manifest is edited as a `yaml.Node` tree
A new `internal/manifest/edit.go` decodes the file into a `yaml.Node` only to locate keys (by line) and to merge:
it finds or creates the top-level `application-capabilities` mapping and sets or skips each capability key. Only
the block is encoded, with the indent the file already uses (two spaces when it has no nested key). The block
replaces its old line range in the original bytes, or is appended at the end, and the lines of
`instance.capabilities` are cut out, so every other byte of the file is unchanged. The file is written to a
temporary file in the same directory and renamed over the original (the target, when the path is a symlink),
keeping its mode.
- Rejected: `Read` plus `Marshal`: drops comments and unknown keys.
- Rejected: encoding the whole node tree: yaml.v3 re-indents every line and drops blank lines.
- Rejected: appending text to the file: cannot merge into an existing block or remove `instance.capabilities`.
- Rejected: appending text to the file: cannot merge into an existing block or remove `instance.capabilities`.

### The schema is generated once and committed
`tools/schemagen` is a small Go program: `go run ./tools/schemagen -spec <file> -out schema/vcr.schema.json`, wrapped
as `make schema SPEC=<file>`. It takes `components.schemas.ApplicationResponse.properties.capabilities` (the one
with all eight), inlines the local `$ref`s, drops `example` and `x-*` keys, turns string boolean defaults into
booleans, sets `additionalProperties: true` on every object, and allows `{{instance_url}}` in addresses by keeping
them plain strings. The hand-written part of the generator holds the rest of the manifest (`project`, `instance`,
`debug`) as a Go literal mirroring the `manifest` structs.
- Rejected: fetching the definition in CI: it is not published at a stable public URL.
- Rejected: a hand-written capabilities schema: 85 KB of definition would drift without a regeneration path.

### One schema test validates the defaults and the corpora
`internal/manifest/schema_test.go` loads `schema/vcr.schema.json` with a JSON Schema validator (test-only
dependency, `github.com/santhosh-tekuri/jsonschema/v6`) and checks: the block produced with every capability flag,
every manifest under `internal/manifest/testdata` and `test/contract/testdata/corpora` that the CLI accepts, and
the failing cases from the `manifest-schema` spec.
- Rejected: a validator in the CLI binary: nothing at run time uses the schema.

### The new commands join the contract harness as additions
The two new nodes go into `contract-tree.json`, `vcr app` gets the new child there, and three help pages are added
or re-recorded (`vcr_app.txt`, `vcr_app_capabilities.txt`, `vcr_app_capabilities_add.txt`), taking
`goldenPages` to 29. Scenarios `app-capabilities-*` cover each flag. They are rich-only terminal scenarios
(`presentation: rich`, recorded from the binary under test), because the baseline release has no such command to
record a classic transcript from. `allowed-diffs.txt` gets one line for the `vcr app` help page.
- Rejected: leaving the commands out of the tree: `TestContractTree` fails on a child not in the baseline.

## Risks / Trade-offs

- [The committed schema goes stale when Vonage adds fields] → Unknown keys pass, so staleness costs completion, not
  errors; `make schema` regenerates it.
- [`meetings` exists only in the response schema and may be retired by Vonage] → It is generated like the others;
  a platform error on deploy is the signal to drop it.
- [yaml.v3 re-encodes the block, which can change its spacing, blank lines or line wrapping] → The risk is confined
  to the block: the test for the spec's "rest of the manifest is kept" requirement compares every other line of a
  commented, four-space corpus file byte for byte.
- [Rich-only scenarios are pinned from the branch, not a baseline] → They are regression pins reviewed like any
  rich golden; there is no older behaviour to compare with.
