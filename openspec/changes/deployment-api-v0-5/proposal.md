# Proposal

## Why

The CLI calls version `v0.3` of the deployment API. Version `v0.5` is served in every region and names each package's image after the app, as `<name>:<n>`, where `v0.3` names it after the package id, as `<packageId>:1`. Moving the CLI to `v0.5` gives a deployed app's images a stable name and a version that counts up with each deploy.

## What Changes

- Releases and CI builds call `/v0.5/...` on the deployment API in place of `/v0.3/...`.
- On `v0.5` and later, `vcr deploy` sends `name` in `POST /packages`: `<project name>-<instance name>` from the manifest or the flags, sanitised to lowercase letters, digits and `-`, a valid Docker image name. On `v0.3` and `v0.4` the body is unchanged.
- A project and instance name that sanitise to nothing end the deploy with exit code 1 before the package is created.
- Every other request, and the output and exit code of every command, is unchanged.

## Capabilities

### New Capabilities

- `package-naming`: what name `vcr deploy` gives the package it creates, and on which API versions.

### Modified Capabilities

None.

## Impact

- Code: the platform client's package request, the deploy request builder.
- Build: the default `API_VERSION` in the `Makefile` and the release and CI workflows, which stop reading the repository variable.
- Tests: none of the contract transcripts change, because the harness stamps `v0.3` on both sides; unit tests cover `v0.5`.

## Branch

`feature/cli-rewrite`: these commits join the open rewrite pull request.

## Rollout

- Every region already serves `v0.5`, so no server change is needed.
- The workflows stop reading the `API_VERSION` repository variable, which is set to `v0.3`. Releases cut from `main` before the rewrite merges keep using the variable and `v0.3`. Delete the variable after the rewrite merges.

## Out of scope

- Changing the `API_VERSION` repository variable: `main`'s CLI would then call `v0.5` without `name`, and `v0.5` requires `name`.
- Checking the name before the upload: the empty-name error comes after the upload, like the other package checks (missing runtime, bad capabilities).
- `vcr debug`: it creates no packages.
