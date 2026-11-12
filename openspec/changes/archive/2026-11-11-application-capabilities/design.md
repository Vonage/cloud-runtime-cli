# Design

## Decisions

1. **The block is `map[string]any` on `Manifest`** (`yaml:"application-capabilities,omitempty"`). yaml.v3
   decodes nested mappings to `map[string]any`, which `encoding/json` sends as written. `omitempty` keeps
   `vcr init`'s written files byte-identical when there is no block. `cloneManifest` deep-copies it.
2. **One validation function in `internal/manifest`** (`CheckApplicationCapabilities(m, flagCapabilities)`)
   runs the exclusivity and token checks, and refuses a non-text mapping key (yaml.v3 decodes one to
   `map[any]any`, which `encoding/json` cannot send). Both commands call it straight after reading the
   manifest, before the region lookup, so no platform call happens first: `vcr deploy` in `run` after
   `manifest.Read`, `vcr debug` in `execute` after `loadProject` (not in `preflight`, which runs after the region
   lookup and the application picker). The token scan walks string values and matches `{{` … `}}`.
3. **The debug guard sits after `confirmApplication`** (`internal/cli/debug/resolve.go:92`): the prompt can
   change the id, so the check uses the final one. It is an error, not a prompt, because prompts are skipped
   with `--yes` and off a terminal, which is where CI runs.
4. **No API-version gate on the new field.** It is `omitempty`, and an older platform ignores unknown fields,
   so it is sent on every version. The contract harness stamps v0.3, so a gate would also hide it from the
   harness.
5. **The platform's response is the proof.** The deploy and debug responses gain an optional
   `applicationCapabilities` object (the block as applied, token replaced). The CLI prints every `address`
   field in it, so it needs no knowledge of Vonage's schema beyond that one field name. Lines use the label
   `Webhook <dotted path>:`, not `Instance host address:`, which the contract's scraped patterns pin. Values go
   through `term.Sanitize`, since they are platform text.
6. **The warning goes to stderr**, like the bundle warnings in `internal/deploy/stages.go:162`.

## Rejected alternatives

- The CLI substitutes `{{instance_url}}` itself: lost because it cannot prove the platform applied anything,
  and the platform has to substitute anyway for installs that never pass through the CLI.
- A schema-aware summary (answer URL, event URL, …): lost because it would bring back the copy of Vonage's
  contract that this feature removes.
- Contract scenarios for the new behaviour: lost because transcripts are recorded from the baseline, which
  lacks the feature, so every new field would need a whole-field allowed-diff that pins nothing. Unit tests
  over fakes pin it; the unchanged contract corpus proves nothing else moved.
- A prompt instead of the debug guard error: lost for the `--yes` and CI reason above.

## Risks

- A typo in a Vonage field name passes the CLI and comes back as the platform's error at deploy, after the
  build. Accepted: validating it would mean keeping Vonage's schema.
