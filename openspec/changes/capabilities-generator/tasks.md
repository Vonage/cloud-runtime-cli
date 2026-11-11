# Tasks

`<WT>` is this worktree, on the same branch as the `application-capabilities` change, which is archived in an earlier
commit; the block, `CheckApplicationCapabilities` and `Manifest.ApplicationCapabilities` come from there. Tests use fakes and temp
directories, never mock generators. Every task works test first: write the test, run it and watch it fail for the
right reason, then write the code. Every task ends with `cd <WT> && go test ./<package>/...` passing for the
packages it touched, and every group ends with `cd <WT> && make test lint` passing before its commit.

## Exact strings

- No flag: `no capability given: pass one or more of --voice, --messages, --rtc, --verify, --video, --vbc, --meetings, --network-apis`
- Network APIs missing value: `--network-apis needs --network-application-id and --redirect-uri`
- Network APIs value without the flag: `--network-application-id and --redirect-uri need --network-apis`
- No manifest: `failed to find template manifest file: manifest file not found (use --print to see the block without a manifest)`
- Skipped (stderr, warning icon): `<cap> is already configured, skipped (use --force to replace it)`
- Added (stderr, success icon): `Added <cap> to application-capabilities in <file>`
- Replaced (stderr, success icon): `Replaced <cap> in application-capabilities in <file>`
- List removed (stderr, warning icon): `Removed instance.capabilities (<entries joined with ", ">): application-capabilities replaces it`

## 1. Defaults

- [x] 1.1 Failing table test in new `internal/manifest/appcaps_defaults_test.go`: `DefaultCapability(name, NetworkAPIs{})`
  returns the default from the spec table for each of the eight names, exact `map[string]any` per name (voice,
  messages with `version: v1`, rtc, verify with `version: v2`, the six video webhooks with `active: true` and no
  `http_method`, `vbc` as an empty map, the three meetings webhooks, network_apis from the struct). An unknown name
  returns an error. Verify: `cd <WT> && go test ./internal/manifest/ -run TestDefaultCapability` fails, then passes.
- [x] 1.2 Implement `internal/manifest/appcaps_defaults.go`: `CapabilityNames` (flag order), `NetworkAPIs{ApplicationID,
  RedirectURI}`, `DefaultCapability`, and the address helper (`{{instance_url}}/<cap>/<webhook>`, `_url` dropped,
  `_` to `-`). Verify: the 1.1 test passes; `CheckApplicationCapabilities` on a block of all eight defaults returns nil
  (add that case to the 1.1 file).

## 2. Manifest editor

- [x] 2.1 Failing tests in new `internal/manifest/edit_test.go` for `AddCapabilities(src []byte, add map[string]any,
  force bool) (out []byte, Result, error)`; `Result` lists added, replaced, skipped and the removed
  `instance.capabilities` entries. Cases: empty block created at the end of the file; merge into an existing block;
  skip without force; replace with force and leave other capabilities alone; `instance.capabilities` removed and its
  entries reported; a corpus file `internal/manifest/testdata/commented.yml` (comments above and beside keys, a
  top-level `custom: 1`, quoted strings) whose every line outside the block and `instance.capabilities` is unchanged
  in the output; a file whose top level is not a mapping returns an error. Verify: `cd <WT> && go test
  ./internal/manifest/ -run TestAddCapabilities` fails, then passes after 2.2.
- [x] 2.2 Implement `internal/manifest/edit.go` on `yaml.Node` (decode, find or append the top-level mapping key,
  set or skip each child, encode only the block with the file's indent and splice it and the `instance.capabilities`
  removal into the original bytes by line range), plus `WriteFileAtomic(path,
  data)` (temp file in the same directory, keep the mode, rename). Test `WriteFileAtomic` keeps a `0600` mode. Verify:
  the 2.1 tests and `cd <WT> && go test ./internal/manifest/...` pass.

## 3. Command

- [x] 3.1 Failing tests in new `internal/cli/app/capabilities_test.go`, using the package's test env and a temp
  project dir: each flag writes its default (one row per flag); the three flag errors from Exact strings exit 1 and
  leave the file byte-identical; skip and `--force` messages on stderr; list removal message; `--filename`; no
  manifest error; `--print` with and without a manifest writes YAML of only the block to stdout and leaves the file
  unchanged and creates none; no config file and no fake platform requests. Verify: `cd <WT> && go test
  ./internal/cli/app/ -run TestCapabilitiesAdd` fails, then passes after 3.2.
- [x] 3.2 Implement `internal/cli/app/capabilities.go` (`vcr app capabilities`, `cmdenv.None`, group only) and
  `capabilities_add.go` (flags from `CapabilityNames`, `--network-application-id`, `--redirect-uri`, `--filename`/`-f`,
  `--force`, `--print`), registered in `app.go` `NewCmd`. Add `capabilities` to the `vcr app` Long text's AVAILABLE
  COMMANDS and one example. Verify: the 3.1 tests pass; `cd <WT> && make build && ./dist/vcr app capabilities add
  --voice --print` prints the voice block.

## 4. Contract harness

- [x] 4.1 Add the `vcr app capabilities` and `vcr app capabilities add` nodes to
  `test/contract/testdata/contract-tree.json`, add `capabilities` to the `vcr app` children, update its Long and
  Example. Verify: `cd <WT> && go test ./internal/cli/ -run TestContractTree` passes.
- [x] 4.2 Add `vcr_app_capabilities.txt` and `vcr_app_capabilities_add.txt` to `testdata/golden-help/`, re-record
  `vcr_app.txt`, set `goldenPages` to 29 in `internal/cli/golden_test.go`, and add one `allowed-diffs.txt` line for the
  `vcr app` help scenario (reason: `vcr app capabilities was added`). Verify: `cd <WT> && go test ./internal/cli/
  -run Golden` passes.
- [x] 4.3 Add rich-only terminal scenarios `test/contract/scenarios/app-capabilities-*.yaml` that together use every
  flag of the new command, record them with `make contract-rich-record SCENARIO='app-capabilities-*'`, and add an
  `app capabilities: app-capabilities-*` row to `coverage.txt`. If the harness cannot run a rich-only scenario for a
  command the baseline lacks, stop and ask. Verify: `cd <WT> && make contract contract-rich contract-baseline-check`
  passes.

## 5. Schema

- [ ] 5.1 Failing test in new `internal/manifest/schema_test.go` (adds the test-only dependency
  `github.com/santhosh-tekuri/jsonschema/v6`): loads `../../schema/vcr.schema.json`; passes the all-eight default
  block wrapped in a minimal manifest, `testdata/commented.yml`, and every manifest under `internal/manifest/testdata`
  and `test/contract/testdata/corpora` that `manifest.Parse` accepts; fails `instance.scaling.min-scale: "two"` and
  `application-capabilities.voice.region: mars-1` at those paths; passes `voice.new_field: 1`. Verify: `cd <WT> && go
  test ./internal/manifest/ -run TestSchema` fails (no file).
- [ ] 5.2 Implement `tools/schemagen/main.go` (`-spec`, `-out`) as the design says, its unit test
  `tools/schemagen/main_test.go` on a small fixture definition (a `$ref` inlined, `example` and `x-*` dropped,
  `default: "true"` made `true`, `additionalProperties: true` set), and a `schema` Makefile target (`SPEC` required,
  added to `.PHONY` and `help`). Generate `schema/vcr.schema.json` from the Application API v2.4.1 definition and
  commit it. Verify: `cd <WT> && go test ./tools/schemagen/ ./internal/manifest/ -run 'TestSchema|.'` passes and
  `make schema SPEC=<file>` run twice leaves `git status` clean.
- [ ] 5.3 Add one paragraph to `README.md` on the schema: the URL
  `https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/schema/vcr.schema.json` and the
  `# yaml-language-server: $schema=<url>` line. Verify: `cd <WT> && make docs` leaves `docs/` matching the new help
  (commit the regenerated `docs/vcr_app*.md`).

## 6. Verify

- [ ] 6.1 `cd <WT> && make build test lint vulncheck tidy contract contract-rich contract-baseline-check` all pass;
  paste the summary lines into the PR. Then in a scratch copy of a real project: `vcr app capabilities add --voice
  --messages` and `vcr deploy` against a dev platform show the `Webhook voice.webhooks.answer_url` line.

## Workflow follow-up

- `openspec validate capabilities-generator --strict`, then archive as the last commit.
- After the release: a pull request to `SchemaStore/schemastore` adding the catalog entry.
