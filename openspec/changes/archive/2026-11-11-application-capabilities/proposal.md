Branch: `feature/application-capabilities`

Sibling change: the platform side ships separately and must be live before this change is released.

## Why

Setting `instance.capabilities` makes the platform point the linked Vonage application's webhooks at
addresses the platform owns, which then forward each webhook to the instance. Customers who want Vonage to
call their instance directly have to edit the application by hand, and the next `vcr deploy` overwrites
the edit. There is no way to say it in `vcr.yml`. The platform is adding support for a raw Vonage application
`capabilities` object, applied as written with the instance's address filled in. The CLI has to read it
from the manifest, send it, and show what was applied.

## What Changes

- A new top-level manifest block, `application-capabilities`, holding the Vonage Applications API
  `capabilities` object as Vonage spells it (snake_case keys). The CLI does not interpret it beyond the
  checks below. The only token is `{{instance_url}}`, which the platform replaces with the instance's
  scheme and host (in `vcr debug`, the debug session's).
- Checks before any platform call, each an error with exit code 1:
  - the block together with `instance.capabilities`, or together with `vcr deploy --capabilities`;
  - a `{{...}}` token other than `{{instance_url}}` anywhere in the block's string values;
  - in `vcr debug`, the block set while the session's application id is `instance.application-id`.
    Debug would otherwise repoint the production application's webhooks at the debug session, and they
    would stay there until the next deploy.
- `vcr deploy` sends the block as `applicationCapabilities` on package create; `vcr debug` sends it on the
  debug-session request. Requests without the block are byte-identical to today's.
- When the platform's response lists the addresses it applied, the deploy summary and the debug intro
  print one line per webhook address. When the CLI sent a block and the response has none, it warns on
  stderr that the region's platform did not apply it.
- `vcr init` keeps an `application-capabilities` block that a template's `vcr.yml` carries; today the
  manifest round trip would drop it.

## Capabilities

### New Capabilities

- `application-capabilities`: the manifest block, its checks, what `vcr deploy` and `vcr debug` send, and
  what they print.

### Modified Capabilities

None. Manifests without the block behave as today.

## Impact

- `internal/manifest` (the field, a validation function, cloning), `internal/cli/deploy`, `internal/deploy`
  (request, summary, warning), `internal/platform` (request and response types), `internal/cli/debug` and
  `internal/debugsession` (guard, request, intro), and their tests.
- The contract corpus is unchanged: every existing scenario must still pass with no new allowed-diff line.
- No new flag, no help text change.

## Rollout

Release after the platform support is live in every region. A release before that is safe: the platform
ignores the field, nothing is configured, and the CLI warns.

## Out of scope

- Editing `vcr deploy --help`: the help pages are pinned to the recorded baseline. The block is documented
  in the public Cloud Runtime docs instead.
- Validating Vonage field names, webhook paths or reachability: the payload is Vonage's contract. A typo
  comes back as the platform's error.
- More tokens, such as the application id: one map entry each, if ever needed.
- Changing `vcr app create` or the `vcr init` templates to use the block: changes behaviour for everyone.
