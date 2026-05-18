# Design: `vcr app remove`

**Date:** 2026-05-18

## Summary

Add a `vcr app remove <applicationID>` command that removes a Vonage application via the deployment API. Follows existing CLI conventions established by `vcr instance remove` and `vcr secret remove`.

## Command Interface

```
vcr app remove <applicationID> [--yes|-y]
```

- `applicationID`: required positional argument (Vonage application UUID).
- `--yes` / `-y`: skip the interactive confirmation prompt (e.g. for CI environments). Prompt is also skipped when `io.CanPrompt()` returns false.
- `rm` alias available (consistent with `vcr instance rm`, `vcr secret rm`).
- Without `--yes`, the CLI prints:
  ```
  Are you sure you want to remove application "<applicationID>"?
  ```
  and aborts on anything other than `y`/`yes`.

## File Structure

Follows the existing pattern under `vcr/app/`:

```
vcr/app/remove/
  remove.go        # NewCmdAppRemove, Options, runRemove
  remove_test.go
```

`vcr/app/app.go` gets a new `AddCommand(removeCmd.NewCmdAppRemove(f))` line.

## API Layer

Add `DeleteVonageApplication` to `pkg/api/deployment.go`:

```go
func (c *DeploymentClient) DeleteVonageApplication(ctx context.Context, appID string) error
// DELETE {baseURL}/applications/{appID}
// Returns nil on 2xx and 404 (idempotent), error on other non-2xx.
```

The `Factory` interface in `pkg/cmdutil/factory.go` exposes `DeploymentClient()` — `DeleteVonageApplication` is added to `DeploymentInterface` and mocks regenerated.

## Output

| Scenario | Output | Exit code |
|---|---|---|
| Success | `✓ Application "<id>" successfully removed` to stdout | 0 |
| User aborts prompt | `! Application removal aborted` to stderr | 0 |
| Other API error | `failed to remove application: <err>` to stderr | 1 |

Note: 404 is treated as success (idempotent delete) — no error surfaced to the user.

## Tests

Table-driven tests in `remove_test.go` using `gomock` and `testutil.NewTestIOStreams()`:

- Successful remove with `--yes` flag (no prompt).
- Successful remove after user confirms at prompt.
- User types `n` at prompt — aborts, no API call made.
- Missing application ID argument — cobra returns error.
- API returns error — command returns wrapped error.

## Consistency Notes

- Command name `remove` / alias `rm` matches `vcr instance remove` and `vcr secret remove`.
- Flag name `--yes` / `-y` matches `vcr instance remove` and `vcr secret remove`.
- Prompt text capitalised: `"Are you sure you want to remove application %q?"`.
- Prompt gated on `io.CanPrompt()` matching existing pattern.
- API method named `DeleteVonageApplication` (HTTP verb) while CLI command is `remove` (user-facing verb).
