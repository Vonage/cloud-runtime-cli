# Design

## Context

- `main.apiVersion` defaults to `v0.3` in `main.go` and is stamped at build time: `Makefile` `API_VERSION ?= v0.3`; `ci.yml` and the unsigned build in `release.yml` use `vars.API_VERSION || 'v0.3'`; the signed build in `release.yml` and the build in `release-please.yml` use `vars.API_VERSION` alone. `internal/platform` uses it only to build the base URL, `<deployment API URL>/<version>`.
- `POST /packages` is `platform.Client.CreatePackage(ctx, PackageRequest)`. `internal/deploy/stages.go` `packageRequest` builds the request; the runner already has `Plan.ProjectName` and `Plan.InstanceName`, with flags applied over the manifest.
- On `v0.5` the server requires `name` and builds the image as `<cleaned name>:<latest version of that name + 1>`. On `v0.3` and `v0.4` it ignores fields it does not know, and names the image `<packageId>:1`.
- The contract harness (`test/contract/build.go`) stamps `v0.3` on both the v2.6.1 baseline and the binary under test.

## Decisions

### Where the version check lives

`PackageRequest` gains `Name string` with `json:"name,omitempty"`. Deploy always fills it with `packageName(ProjectName, InstanceName)`. `Client.CreatePackage` clears it when the client's API version is below `v0.5` (`golang.org/x/mod/semver`, already a dependency). Only the platform package knows the version, so the check stays there.

- Always sending `name`, on every version: lost, because it changes the request of all 80 contract scenarios that create a package, and the replay would then no longer check the rest of those requests.
- Bumping the harness stamp to `v0.5`: lost, because every request path of every scenario would differ from the v2.6.1 transcripts.

### Sanitising

`packageName` in `internal/deploy` joins the two names with `-`, lowercases, turns each run of characters outside `a-z0-9` into one `-` and trims `-` from both ends. The result is always a valid Docker image name component (lowercase letters, digits and separators, no separator at either end). Deploy owns it because the name is a deploy decision, not a wire detail. An empty result is an error from `packageRequest`, next to the existing runtime check, so it is reported after the upload like them. The server still cleans the name; on our output that is a no-op.

- Keeping `.` and `_`, which Docker allows: lost, because only letters, digits and `-` were asked for.
- Leaving repeated or edge dashes: lost, because Docker refuses a name that starts or ends with a separator.

### The workflows stop reading the repository variable

Every build in `ci.yml`, `release.yml` and `release-please.yml` sets `API_VERSION: v0.5` directly, and `main.go` defaults to `v0.5` for a plain `go build`. `main` runs its own copies of these workflows, so its releases keep reading the variable until the rewrite merges. Changing the repository variable would also change releases cut from `main`, whose CLI does not send `name`.

### Docs and fixtures

`docs/docs.go` and `AGENTS.md` say `v0.5`. `cmdtest`'s `testAPIVersion` stays `v0.3`, so the existing command tests keep checking the unchanged requests; new tests stamp `v0.5` explicitly.
