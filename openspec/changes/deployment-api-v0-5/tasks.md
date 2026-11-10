# Tasks

Run every command from the repository root. `make test` is `go test -race ./...`. Each task is one commit.

## 1. Send the package name on v0.5

- [x] 1.1 `PackageRequest.Name`, sent only from `v0.5`.
  - Files: `internal/platform/deployments.go`, `internal/platform/deployments_test.go` (or the existing test file for `CreatePackage`).
  - Test first: with a client built for `v0.5`, `CreatePackage` with `Name: "shop-dev"` sends a body with `"name":"shop-dev"`; with a client built for `v0.3` or `v0.4`, the same call sends a body with no `name` key.
  - Verify: `go test ./internal/platform/ -count=1`
  - Accept: the new tests pass and the existing `CreatePackage` tests are unchanged.

- [x] 1.2 Deploy names the package `<project>-<instance>`, sanitised.
  - Files: `internal/deploy/packagename.go` (`packageName`), `internal/deploy/packagename_test.go`, `internal/deploy/stages.go` (`packageRequest`), `internal/deploy/*_test.go`, `internal/cli/deploy/env_test.go`, `internal/cli/deploy/fake_test.go`.
  - Test first: table test for `packageName`: `web`/`prod` → `web-prod`; `My_Shop 2`/`Dev.EU` → `my-shop-2-dev-eu`; `--a--`/`b` → `a-b`; `123`/`456` → `123-456`; `__`/`..` → empty. Then: a plan with `ProjectName: "web"`, `InstanceName: "prod"` builds `Name: "web-prod"`; a plan whose names sanitise to empty fails in `packageRequest` with an error naming both; a deploy command test (`internal/cli/deploy`, `newEnvAt`) built for `v0.5`, with a manifest naming `Shop` and `dev`, sends `POST /v0.5/packages` with `"name":"shop-dev"`.
  - Verify: `go test ./internal/deploy/ ./internal/cli/deploy/ -count=1`
  - Accept: tests pass, and `make contract` passes with no new lines in `test/contract/allowed-diffs.txt`.

## 2. Build for v0.5

- [ ] 2.1 Default `API_VERSION` to `v0.5`.
  - Files: `Makefile`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `docs/docs.go`, `AGENTS.md`, `internal/buildinfo/buildinfo.go` and `internal/platform/options.go` (comments).
  - Test first: none; run `make build && ./dist/vcr --version` and check that `strings dist/vcr | grep -c '^v0.5$'` is at least 1.
  - Verify: `make build test lint docs` and `make contract`
  - Accept: everything passes; `grep -rn "v0\.3" Makefile .github/workflows docs/docs.go AGENTS.md` prints nothing.
