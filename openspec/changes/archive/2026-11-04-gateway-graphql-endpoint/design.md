# Design

## Context

`--region` is checked by a pattern (`aws.<name>`) in the root command, then turned into a host by string formatting. The host carried the region name, so any well-formed name produced a URL. The gateway host instead depends on the region's geography, which the name alone does not tell us.

## Goals / Non-Goals

**Goals:**
- One table in the CLI maps each supported region to its gateway geography, and both validation and URL building use it.

**Non-Goals:**
- Discovering regions at runtime.

## Decisions

- **A closed table drives validation.** The table holds only the client-facing primary regions; standby regions are left out even though the gateway routes them. `IsValidRegion` becomes "is in the table", so the existing root-command check rejects unknown regions with no new code path, and a region still can never choose the host.
  - Rejected: keep the pattern check and fall back to the old host for unknown regions, because it keeps two hosts in play and hides a missing table entry.
- **Notarization behind an opt-in variable.** The three notarization steps run only when `vars.APPLE_NOTARIZE == 'true'`. A releaser doesn't need to write any setting to ship, and turning it back on is one variable.
  - Rejected: deleting the steps, because restoring them later means re-reviewing the workflow.
  - Rejected: an opt-out variable, because every release would need a settings write first.

## Risks / Trade-offs

- [A new region needs a CLI release] → Users can pass `--graphql-endpoint` or set `graphql_endpoint` meanwhile.
- [Unnotarized macOS binaries are blocked when downloaded through a browser] → Release notes say so; the checksums stay signed.
