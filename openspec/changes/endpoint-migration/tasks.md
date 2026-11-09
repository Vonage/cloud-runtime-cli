# Tasks

Run every command from the repository root. `make test` is `go test -race ./...`. Each task is one commit.

## 1. Config file: the endpoint key

- [x] 1.1 Read and write `endpoint`.
  - Files: `internal/config/config.go` (`File.Endpoint`), `internal/config/layout.go` (`keyEndpoint`, `fileFrom`, `applyTo`), `internal/config/read_test.go`, `internal/config/write_test.go`.
  - Test first: `Read` of a file with `endpoint = https://e.example.test/api/deployment` returns it in `File.Endpoint`; `Write` of a `File` with `Endpoint` set puts `endpoint` after `default_region`; `TestWriteLayoutMatchesBaseline` still passes unchanged for a `File` without `Endpoint`.
  - Verify: `go test ./internal/config/ -count=1`
  - Accept: the new tests pass and the existing layout tests are untouched.

- [x] 1.2 `config.AddEndpoint(path, url string) error`.
  - Files: `internal/config/write.go` (or a new `internal/config/endpoint.go`), `internal/config/write_test.go`.
  - Test first: on a file with comments, an extra section and quoted credentials, `AddEndpoint` adds only `endpoint = <url>` and every other byte of the original lines is kept; mode stays 0600; it writes through a symlink to the target; a read-only file returns an error and is unchanged.
  - Verify: `go test ./internal/config/ -count=1`
  - Accept: tests pass on macOS and with `GOOS=windows go vet ./internal/config/`.

## 2. Bootstrap: precedence and source

- [x] 2.1 `ConfigEndpoint` input and the reported source.
  - Files: `internal/endpoint/bootstrap.go`, `internal/endpoint/endpoint_test.go`.
  - Test first: table cases for the order `Endpoint` > `GraphQLFlag` > `Region` > `ConfigEndpoint` > `GraphQLConfig` > default, using the spec's "Endpoint precedence" scenarios; each case also asserts the returned source, the mapped URL and whether the GraphQL URL was the production form.
  - Verify: `go test ./internal/endpoint/ -count=1`
  - Accept: tests pass; `Bootstrap`'s doc comment shows the new order.

- [x] 2.2 Pass the file's `endpoint` everywhere `Bootstrap` is called.
  - Files: `internal/cli/prerun.go` (`initPlatform`), `internal/cli/configure/configure.go` (`newResolver`), `internal/cli/prerun_test.go`.
  - Test first: in `internal/cli/prerun_test.go`, a `full` probe with a config whose `endpoint` is the fake's deployment URL and whose `graphql_endpoint` is an unmappable URL succeeds and asks the fake.
  - Verify: `go test ./internal/cli/... -count=1`
  - Accept: test passes.

## 3. Notices and the prompt

- [x] 3.1 Notice for `--graphql-endpoint`.
  - Files: `internal/cli/prerun.go` (new `migrateEndpoint` step after `loadConfig`, before `startConnecting`), a new `internal/cli/endpoint_migration_test.go`.
  - Test first: with the harness (`newHarness`), `full` with `--graphql-endpoint <gateway form>` prints exactly `! --graphql-endpoint is deprecated; use --endpoint <mapped> instead` on stderr, stdout and exit code equal to the same run with `--endpoint <mapped>`; the production form prints the `--region` line; in `runRich(term.Rich, ...)` nothing is asked; with a config holding only `graphql_endpoint`, only the flag notice appears.
  - Verify: `go test ./internal/cli/ -run Endpoint -count=1`
  - Accept: tests pass; no notice when `--endpoint` or the file's `endpoint` decides the URL.

- [x] 3.2 Ask or tell for `graphql_endpoint` in the file.
  - Files: `internal/cli/prerun.go`, `internal/cli/endpoint_migration_test.go`.
  - Test first:
    - classic (`h.run`): stderr has the `add "endpoint = <url>"` line naming the file; the file is unchanged; stdout and exit code as before.
    - rich and static (`runRich`) with stdin `y\n`: the question appears on stdout before any connecting step output, the file gains `endpoint`, `✓ Added endpoint to <path>` is printed, the probe runs.
    - stdin `n\n` and empty stdin: file unchanged, probe runs.
    - read-only file and `y`: one `! Could not update` line on stderr, probe runs, exit 0.
    - the file already has `endpoint`: nothing printed.
    - a `none` probe (`plain`) and `completion bash`: nothing printed, file unchanged.
  - Verify: `go test ./internal/cli/ -run Endpoint -count=1`
  - Accept: every case passes. Add a `term.TestRich` variant with stdin if the helper has none (in `internal/term/testrich.go`), with its own test.

## 4. Configure and help

- [x] 4.1 `vcr configure` writes `endpoint`.
  - Files: `internal/cli/configure/configure.go` (`fileToWrite`), `internal/cli/configure/configure_test.go`.
  - Test first: extend `TestConfigureWritesTheFile`, `TestConfigureWritesTheGraphqlEndpointFlag` and `TestConfigureWritesTheEndpointOfTheRegionFlag` to expect `endpoint` equal to the bootstrap URL used, alongside the unchanged `graphql_endpoint`.
  - Verify: `go test ./internal/cli/configure/ -count=1`
  - Accept: tests pass.

- [x] 4.2 Show `--endpoint`, mark `--graphql-endpoint` deprecated.
  - Files: `internal/cli/root.go` (`bindGlobalFlags`), `internal/cli/root_test.go`, `internal/cli/tree_test.go` (`TestContractTreeCarriesEndpoint`: `Hidden: false`), `test/contract/testdata/contract-tree.json`, `docs/docs_test.go` (drop `TestGeneratedPagesOmitHiddenFlags`'s `--endpoint` check), `docs/vcr*.md`, the help goldens under `internal/cli/`.
  - Test first: update `root_test.go`'s flag table to `hidden=false` for `endpoint` and the new `graphql-endpoint` usage `Deprecated: use --endpoint. GraphQL endpoint used to fetch metadata`.
  - Verify: `make docs && go test ./internal/cli/... ./docs/ -count=1`
  - Accept: tests pass; `git diff --stat docs/` shows every page gained the `--endpoint` line.

## 5. Contract suite and docs

- [x] 5.1 Keep unrelated scenarios identical, allow the intended diffs.
  - Files: `test/contract/sandbox.go` (`defaultConfig` gains `endpoint = ${FAKE}/v1/vcr/euw1/api/deployment`), `test/contract/allowed-diffs.txt`.
  - Test first: run `make contract NO_GRAPHQL=1` and list the failing scenarios; each must be a help scenario, a `--graphql-endpoint` scenario, a scenario with its own `config-file` holding only `graphql_endpoint`, or a `configure-*` scenario. Add one line per scenario and field with the reason "the CLI moves users from the GraphQL settings to endpoint". Any other failure is a bug to fix, not to allow.
  - Verify: `make contract NO_GRAPHQL=1 && make contract-rich`
  - Accept: both pass; `allowed-diffs.txt` grows only by those lines.

- [x] 5.2 README.
  - Files: `README.md` (the Usage section).
  - Test first: none (docs only).
  - Verify: `go test ./internal/term/ -run Readme -count=1`
  - Accept: one short paragraph says `endpoint` / `--endpoint` replace `graphql_endpoint` / `--graphql-endpoint`, and that `vcr` offers to add the key.

## 6. Whole-repo checks

- [x] 6.1 Run everything.
  - Verify: `make lint && make test && make contract NO_GRAPHQL=1 && make contract-rich && make vulncheck && GOOS=windows go vet ./...`
  - Accept: all pass. Then by hand, with a config file holding only a gateway `graphql_endpoint`: `vcr app list` on a terminal asks and, after `y`, the file has both keys; `vcr app list | cat` prints the stderr line and asks nothing.
