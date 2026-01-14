# Vonage cloud runtime - CLI

![Actions](https://github.com/Vonage/vonage-cloud-runtime-cli/workflows/Release%20CLI/badge.svg)

<img src="https://developer.nexmo.com/assets/images/Vonage_Nexmo.svg" height="48px" alt="Nexmo is now known as Vonage" />

Vonage cloud runtime - CLI (VCR) is a powerful command-line interface designed to streamline
and simplify the development and management of applications on
the [Vonage Cloud Runtime platform](https://developer.vonage.com/en/cloud-runtime). It is still under active development. Issues, pull requests and other input is very welcome.

* [Installation](#installation)
* [Project Structure](#project-structure)
* [Usage](#usage)
* [Development Setup](#development-setup)
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


## Project Structure

[Structure](PLAN.md) of the project

## Usage

Usage examples are in the `docs/` folder - [vcr](docs/vcr.md)

## Development Setup

### Prerequisites

- Go 1.24+
- Node.js 18+ (for commitlint)
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [Lefthook](https://github.com/evilmartians/lefthook) (for git hooks)

### Setting Up Git Hooks

This project uses [Lefthook](https://github.com/evilmartians/lefthook) to manage git hooks. Install and configure it:

```bash
# Install lefthook (macOS)
brew install lefthook

# Or with Go
go install github.com/evilmartians/lefthook@latest

# Install commitlint dependencies
npm install

# Install the git hooks
lefthook install
```

### What the Hooks Do

| Hook | Action |
|------|--------|
| `commit-msg` | Validates commit message follows [conventional commits](#conventional-commits) format |
| `pre-commit` | Runs `gofmt` and `go vet` on staged Go files |
| `pre-push` | Runs `go test` and `golangci-lint` before pushing |

### Skipping Hooks (when needed)

```bash
# Skip all hooks
git commit --no-verify -m "wip: work in progress"

# Skip specific hooks
LEFTHOOK_EXCLUDE=pre-push git push
```

### Local Hook Overrides

Create a `lefthook-local.yml` to override hooks locally (this file is gitignored):

```yaml
# lefthook-local.yml
pre-push:
  commands:
    go-test:
      skip: true  # Skip tests on push locally
```

## Contributions

Yes please! This command-line interface is open source, community-driven, and benefits greatly from the input of its users.

Please make all your changes on a branch, and open a pull request, these are welcome and will be reviewed with delight! If it's a big change, it is recommended to open an issue for discussion before you start.

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
3. **Publish release**: When the Release PR is merged, a GitHub Release is created
4. **Build & distribute**: The release triggers the build pipeline which:
   - Cross-compiles binaries for all platforms
   - Code signs and notarizes macOS binaries
   - Uploads artifacts to the GitHub Release

### Manual Release

If needed, you can trigger a manual release via the GitHub Actions UI:
1. Go to Actions → "Release CLI"
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