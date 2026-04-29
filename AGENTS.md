# AGENTS.md

Go CLI for the Vonage Cloud Runtime platform. Built with Cobra + resty. Node.js is present **only** for commitlint — this is not a Node project.

## Commands

```sh
make build          # go build -o vcr -v
make test           # go test -v ./...
make test-unit      # alias for test
make test-integration  # requires Docker; runs docker compose in tests/integration/
make test-all       # unit + integration
golangci-lint run   # lint (v2.6.2 — match CI version)
go generate ./...   # regenerate mocks (see Codegen below)
```

Run a single package test:
```sh
go test -v ./vcr/app/list/...
go test -v -run TestMyFunc ./pkg/cmdutil/...
```

## Architecture

- `main.go` — wires `cmdutil.NewDefaultFactory(...)`, creates root command, executes.
- `pkg/cmdutil/factory.go` — central `Factory` interface; all commands receive it.
- `vcr/<command>/` — command implementations mirror the CLI path (`vcr/app/list/` = `vcr app list`).
- `pkg/api/` — API clients (Asset, Deployment, Release, Datastore, Websocket, GraphQL).
- `pkg/config/` — INI (`~/.vcr-cli`) and YAML (`vcr.yaml`) parsing.
- `pkg/format/` — output formatting, tables, error printing.
- `testutil/mocks/factory.go` — generated `Factory` mock used by all command tests.

## Codegen

Mocks are generated from `pkg/cmdutil/factory.go`:
```sh
go generate ./...
```
Output: `testutil/mocks/factory.go`. Regenerate whenever `Factory` interface changes.

## Build Flags

Five `ldflags` variables are injected at build time in CI (`main.apiVersion`, `main.version`, `main.buildDate`, `main.commit`, `main.releaseURL`). A plain `make build` omits them — that is fine for local dev.

## Testing Conventions

- Table-driven tests with `gomock` + `testify/assert`.
- Use `testutil.NewTestIOStreams()` for capturing stdout/stderr in command tests.
- Use `testutil/mocks.NewMockFactory(ctrl)` as the command factory in tests.
- HTTP calls are mocked with `httpmock` (no real network in unit tests).
- Integration tests require Docker and build a Linux amd64 `vcr-cli` binary and a `mockserver` binary into `tests/integration/bin/` before running Docker Compose. Use `make test-integration-build` then `make test-integration`.

## Linting

Config: `.golangci.yml`, golangci-lint **v2.6.2** (same version as CI — mismatches cause false failures).

Notable rules:
- `gocyclo` / `cyclop` max complexity: 25.
- Test files are exempt from `funlen`, `goconst`, `dupl`, `errcheck`, cyclomatic checks.
- Unchecked `Close()` and `fmt.Fprint*` errors are excluded globally.

## Git Hooks (Lefthook)

`lefthook.yml` defines:
- `commit-msg`: `npx commitlint` — requires Node + `npm install` in repo root.
- `pre-commit`: `gofmt -l -w` + `go vet ./...`.
- `pre-push`: `go test ./...` + `golangci-lint run`.

Install hooks: `lefthook install` (after `npm install`).

## Commit Convention

Conventional Commits enforced. Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`. Breaking changes: `feat!:` or `BREAKING CHANGE:` footer.

## Configuration Precedence (Runtime)

Flags > `vcr.yaml` (project manifest) > `~/.vcr-cli` (INI, stores `api_key`, `api_secret`, `default_region`, `graphql_endpoint`).

## Release

- Automated via Release Please on merge to `main`.
- macOS binaries are code-signed ("Developer ID Application: Nexmo Inc.") and notarized — this only runs in GitHub Actions with org secrets. Local builds do not require signing.

## Reference

- `.github/copilot-instructions.md` — detailed patterns for commands, tests, error handling, API clients, output formatting.
