# Vonage cloud runtime - CLI

![Actions](https://github.com/Vonage/vonage-cloud-runtime-cli/workflows/CI-Pipeline/badge.svg)

<img src="https://developer.nexmo.com/assets/images/Vonage_Nexmo.svg" height="48px" alt="Nexmo is now known as Vonage" />

Vonage cloud runtime - CLI (VCR) is a powerful command-line interface designed to streamline
and simplify the development and management of applications on
the [Vonage Cloud Runtime platform](https://developer.vonage.com/en/cloud-runtime). It is still under active development. Issues, pull requests and other input is very welcome.

* [Installation](#installation)
* [Project Structure](#project-structure)
* [Usage](#usage)
* [Terminal output](#terminal-output)
* [Development](#development)
* [Contributions](#contributions)
* [Conventional Commits](#conventional-commits)
* [Release Process](#release-process)
* [Getting Help](#getting-help)

## Installation

Find current and past releases on the [releases page](https://github.com/Vonage/vonage-cloud-runtime-cli/releases).

### macOS

```
curl -s -L https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/script/install.sh | sh
```

### Linux
```
curl -s -L https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/script/install.sh | sh
```

### Windows
```
pwsh -Command "iwr https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/script/install.ps1 -useb | iex"
```

### Verifying a download

From v2.6.1 each release has a `checksums.txt` and a signature of it, `checksums.txt.<keyid>.sig`. The first line of `checksums.txt` is `# vcr <tag>`, followed by one SHA-256 line per archive. The signature is made with `ssh-keygen -Y sign` in the `vcr-release` namespace, and `<keyid>` is the first 8 bytes, in hex, of the SHA-256 of the raw public key. A release can carry one signature file per key; one valid signature is enough. `vcr upgrade` and the install scripts check the checksum and the signature for you, and install nothing that does not verify.

To check a download by hand you need OpenSSH 8.0 or newer. Save the release key as `allowed_signers`:

```
# key id fd1652cb1b7cc0c3
vcr-release namespaces="vcr-release" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB6MAISQBLbNYWna92jPv0bjwWW4uNkeF7cQeJ1PN5FI
```

Then, next to the downloaded files:

```
ssh-keygen -Y verify -f allowed_signers -I vcr-release -n vcr-release -s checksums.txt.<keyid>.sig < checksums.txt
grep ' vcr_linux_amd64.tar.gz$' checksums.txt | sha256sum -c -    # macOS: shasum -a 256 -c -
```

The first command prints `Good "vcr-release" signature` and the second prints `OK`. On Windows, compare `(Get-FileHash vcr_windows_amd64.tar.gz).Hash` with the matching line in `checksums.txt`.

## Project Structure

The layout of the code, the rules it follows and how to check a change are in [AGENTS.md](AGENTS.md).

## Usage

Usage examples are in the `docs/` folder - [vcr](docs/vcr.md)

The platform address is set with `endpoint` in `~/.vcr-cli` or with `--endpoint`. They replace `graphql_endpoint` and `--graphql-endpoint`, which still work but are deprecated. When `vcr` finds only `graphql_endpoint` in the config file, it offers to add the `endpoint` key on an interactive terminal, and otherwise prints the line to add. `vcr configure` writes both keys.

A JSON Schema for `vcr.yml` is published at `https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/schema/vcr.schema.json`. It describes `project`, `instance`, `debug` and `application-capabilities`, and accepts keys it does not describe. Editors that use the YAML language server (for example VS Code with the YAML extension) complete and check the manifest when its first line is `# yaml-language-server: $schema=https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/schema/vcr.schema.json`.

## Terminal output

In an interactive terminal, `vcr` shows live progress that settles into result lines with timings, titled result blocks and compact error messages with hints. Output that is piped, redirected or produced in CI is the same as in earlier releases, so scripts and editor integrations keep working.

The new presentation is used when stdout and stderr are both terminals, `TERM` is set and is not `dumb`, and `CI` is unset (or `0` or `false`). Two settings change it:

| Setting | Effect |
|---|---|
| `VCR_UI=classic` | The output of earlier releases, exactly. Use it for anything that allocates a terminal but reads the output, such as an `expect` script. |
| `VCR_UI=static` | The same lines and layout without animation or cursor movement, for screen readers and recordings. `vcr deploy` prints the build script's lines as they arrive. Also used when the terminal is narrower than 40 columns. |

In the interactive (rich) presentation only, not with `VCR_UI=static`, `vcr deploy` shows a phase bar and the last lines of the build script's output, and collapses them when the build succeeds. In both, a failed build prints the full build log.

`NO_COLOR` turns colour off and keeps the layout. Help pages are not affected.

## Development

Prerequisites: Go 1.27 or newer, `make`, and Node.js 20 for the commit-message hook.

```sh
make hooks      # git hooks: gofmt report and lint on commit, commit-message check, tests on push (once per clone; needs lefthook and Node.js, and also runs `npm ci --ignore-scripts` for the commit-message tool)
make build      # dist/vcr, stamped like a release
make test       # go test -race ./...
make lint       # pinned golangci-lint
make contract   # replay the recorded command-line contract against the built binary
make contract-rich  # replay the terminal goldens
make vulncheck  # govulncheck ./...
make docs       # regenerate docs/vcr*.md
make help       # every target
```

The command-line contract is frozen: `test/contract` holds scenarios and transcripts recorded from v2.6.1, and
`make contract` replays them against the binary built from the working tree. A deliberate difference is a line in
`test/contract/allowed-diffs.txt` with a plain reason; `make contract-record` re-records the transcripts from
`BASELINE_REF` (default `v2.6.1`). Layout and conventions are in [AGENTS.md](AGENTS.md). `git commit --no-verify`
skips the hooks; CI runs the same checks.

## Contributions

Yes please! This command-line interface is open source, community-driven, and benefits greatly from the input of its users.

Please work on a branch and open a pull request; these are welcome and will be reviewed with delight! If it's a big change, it is recommended to open an issue for discussion before you start.

All changes require tests to go with them.

## Conventional Commits

This project uses [Conventional Commits](https://www.conventionalcommits.org/) to automate versioning and changelog generation. All commit messages must follow this format:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Commit Types

| Type | Description | Version Bump |
|------|-------------|--------------|
| `feat` | A new feature | Minor (1.0.0 → 1.1.0) |
| `fix` | A bug fix | Patch (1.0.0 → 1.0.1) |
| `docs` | Documentation only changes | None |
| `style` | Code style changes (formatting, etc.) | None |
| `refactor` | Code refactoring | None |
| `perf` | Performance improvements | Patch |
| `test` | Adding or updating tests | None |
| `build` | Build system changes | None |
| `ci` | CI configuration changes | None |
| `chore` | Other changes | None |
| `revert` | Reverts a previous commit | Varies |

### Breaking Changes

To indicate a breaking change, add `!` after the type or include `BREAKING CHANGE:` in the footer:

```
feat!: redesign configuration format

BREAKING CHANGE: The config file format has changed from YAML to JSON.
```

Breaking changes trigger a major version bump (1.0.0 → 2.0.0).

### Examples

```bash
# Feature
git commit -m "feat: add support for custom deployment regions"

# Bug fix
git commit -m "fix: resolve timeout issue in debug mode"

# Documentation
git commit -m "docs: update installation instructions for Windows"

# Breaking change
git commit -m "feat!: change default API version to v2"
```

## Release Process

This project uses [Release Please](https://github.com/googleapis/release-please) to automate releases. Here's how it works:

1. **Merge to main**: When PRs are merged to `main`, Release Please analyzes the commits
2. **Release PR**: A "Release PR" is automatically created/updated with:
   - Version bump based on commit types
   - Auto-generated changelog entries
3. **Draft release**: When the Release PR is merged, lint and the tests run, then a draft GitHub Release is created
4. **Build, sign & publish**: The release triggers the build pipeline which:
   - Cross-compiles binaries for all platforms
   - Code signs macOS binaries, and notarizes them when the repository variable `APPLE_NOTARIZE` is `true`
   - Signs `checksums.txt` with the release key (see [Verifying a download](#verifying-a-download))
   - Uploads the archives, the checksums and the signature to the draft, then publishes it
   - Installs the published release and checks that it reports the new version

### Manual Release

If needed, you can trigger a manual release via the GitHub Actions UI:
1. Go to Actions → "Manual Release"
2. Click "Run workflow"
3. Enter the release tag (e.g., `v2.1.3`)

## Getting Help

We love to hear from you so if you have questions, comments or find a bug in the project, let us know! You can either:

* Open an [issue on this repository](https://github.com/Vonage/vonage-cloud-runtime-cli/issues)
* Tweet at us! We're [@VonageDev on Twitter](https://twitter.com/VonageDev)
* Or [join the Vonage Community Slack](https://developer.nexmo.com/community/slack)

## License

This library is released under the [Apache 2.0 License][license]

[license]: LICENSE