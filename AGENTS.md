# AGENTS.md

`vcr`, the Vonage Cloud Runtime CLI: a Go (cobra) client for the platform's deployment API.
[`README.md`](README.md) is how to install and use it; this file is for whoever changes it next.

## Layout

- `main.go` — stamps the build info and runs `cli.Run`.
- `internal/cli/` — the command tree; cobra lives only here (and in `docs/`, which generates the reference). One
  package per group: `app`, `secret`, `instance`, `deploy`, `debug`, `initcmd`, `configure`, `upgrade`; `cmdenv` is
  what every command receives and `cmdtest` is the shared test support.
- `internal/platform/` — the only client of the platform; `internal/selfupdate/` — release lookup, signature
  verification, upgrade.
- `internal/term/` — the only package that writes to or reads from the terminal.
- `internal/{config,endpoint,manifest,bundle,deploy,scaffold,debugsession,clierr,clock,buildinfo}` — one noun each; the
  package comment says what it owns.
- `test/contract/` — the frozen command-line contract; `test/fakeplatform/` — the in-process fake platform and fake
  GitHub releases. `docs/` — generated command reference. `script/` — installers. `.github/` — CI and release workflows.

## Facts an agent cannot infer

- The command-line contract is frozen. Recorded transcripts of v2.6.1 decide behaviour: stdout, stderr, exit code,
  written files, requests. A deliberate difference is one line in `test/contract/allowed-diffs.txt` with a plain
  reason; never edit a transcript by hand.
- Some names are public contract and stay: the `X-Neru-*` headers, `~/.neru-cli`, and the `--graphql-endpoint` flag
  (still accepted; it is mapped to the deployment API's address).
- `version`, `commit`, `buildDate`, `apiVersion` and `releaseURL` are stamped with `-X main.*`. `apiVersion` is `v0.5`
  and `make build` fails if it is empty. `upgradeTestKey` is empty in releases.
- Diagnostics keep today's streams (some warnings go to stdout). Do not "fix" that.
- On an interactive terminal (stdout and stderr are terminals, `TERM` is set and not `dumb`, `CI` is unset, `0` or
  `false`, `VCR_UI` is not `classic`) commands use a richer presentation; `VCR_UI=static` drops the animation. Everything
  else, and `VCR_UI=classic`, is byte-identical to v2.6.1; the rich goldens in `test/contract/testdata/rich/` pin it.
- Release asset names are contract: `vcr upgrade` picks `vcr_<os>_<arch>.tar.gz` by exact name, like old clients. It
  changes nothing unless `checksums.txt` verifies against an SSH signature (namespace `vcr-release`) of a key listed in
  `internal/selfupdate/keys.go` and the archive matches it.
- This repository is public. No ticket keys, internal hostnames, internal repository names or planning ids in code,
  tests, comments, commits or pull-request text.

## Rules

1. Credentials travel only over https (or to loopback), checked on every redirect hop (`internal/platform`).
2. Text from the platform or other parties is sanitised before it reaches a terminal (`internal/term`).
3. Nothing is written outside the project directory (never its `.git`), the config file's real path, the installed
   binary's directory or the temp directory.
4. The local debugger answers only local, non-browser callers (`internal/debugsession`).
5. `vcr upgrade` installs nothing without a valid signature (`internal/selfupdate`).
6. Known vulnerabilities are scanned for on every pull request and weekly (`make vulncheck`); a finding blocks the pull
   request unless it carries the `vulncheck-override` label.

Shape: files under 400 lines, small functions, errors returned and rendered once by `clierr.Render`, comments only
where the code is not obvious, interfaces declared by the consumer, tests over fakes and `httptest` (no mock
generators), test names that describe behaviour.

## Verification

```sh
make hooks      # git hooks, once per clone (needs lefthook and Node.js)
make build      # dist/vcr, stamped like a release
make test       # go test -race ./...
make lint       # pinned golangci-lint, native and windows
make contract   # replays every scenario against the built binary; make contract-record re-records from BASELINE_REF (v2.6.1)
make contract-rich  # replays the rich terminal goldens; make contract-rich-record re-records them
make vulncheck  # govulncheck ./...
make docs       # regenerates docs/vcr*.md; a test fails when they are stale
make help       # every target
```

Conventional commit subjects; the pull-request title becomes the changelog entry.
