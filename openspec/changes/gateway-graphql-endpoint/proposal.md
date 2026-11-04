# Proposal

Branch: `feature/gateway-graphql-endpoint`

## Why

The CLI reaches the platform's GraphQL API on a per-region `runtime.vonage.cloud` host, while the supported public entry point is the Vonage API gateway (`api-<geo>.vonage.com/v1/vcr/<region>/api/graphql`). New configs and `--region` should use the gateway, as working configs already do by hand.

## What Changes

- `vcr configure` writes the EU gateway GraphQL URL as the default `graphql_endpoint` when none is set.
- `--region aws.<name>` builds the gateway GraphQL URL for that region: `euw1` on `api-eu.vonage.com`, `use1` on `api-us.vonage.com`, `apse1`/`apse2` on `api-ap.vonage.com`.
- **BREAKING**: `--region` with any other region, including the standby regions `euc1` and `usw2`, is rejected with an error listing the supported regions. Before, any well-formed `aws.<name>` was accepted.
- An explicit `--graphql-endpoint` or an existing `graphql_endpoint` in the config file keeps working unchanged.
- Release tooling: macOS notarization runs only when the repository variable `APPLE_NOTARIZE` is `true`, so releases can ship while notarization is unavailable. Binaries are still code-signed.

## Capabilities

### New Capabilities
- `graphql-endpoint`: how the CLI chooses the GraphQL endpoint it talks to.

### Modified Capabilities

## Impact

- `pkg/config/config.go`, `pkg/cmdutil/factory.go`, `vcr/root/root.go` and their tests.
- `.github/workflows/release-please.yml`, `.github/workflows/release.yml`.

## Rollout

Ships in the next release. Existing config files are not rewritten. Until `APPLE_NOTARIZE` is set, macOS archives are signed but not notarized: files downloaded through a browser need the quarantine flag removed; the install script and `vcr upgrade` are unaffected.

## Out of scope

- Rewriting existing `graphql_endpoint` values in users' config files: they still work, and silent rewrites of user files are risky.
- Making `vcr configure` replace an old endpoint it finds: it keeps whatever the user set.
- Deriving the gateway host from the region list the API returns: the API is reached through the endpoint being chosen, so the table has to live in the CLI.
