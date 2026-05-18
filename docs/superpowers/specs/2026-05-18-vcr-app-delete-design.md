# Design: `vcr app delete`

**Date:** 2026-05-18

## Summary

Add a `vcr app delete <applicationID>` command that deletes a Vonage application via the deployment API. Follows existing CLI conventions established by `vcr instance remove` and `vcr secret remove`.

## Command Interface

```
vcr app delete <applicationID> [--yes|-y]
```

- `applicationID`: required positional argument (Vonage application UUID).
- `--yes` / `-y`: skip the interactive confirmation prompt (e.g. for CI environments). Prompt is also skipped when `io.CanPrompt()` returns false.
- Without `--yes`, the CLI prints:
  ```
  are you sure you want to delete application "<applicationID>"? [y/N]
  ```
  and aborts on anything other than `y`/`yes`.

## File Structure

Follows the existing pattern under `vcr/app/`:

```
vcr/app/delete/
  delete.go        # NewCmdAppDelete, Options, runDelete
  delete_test.go
```

`vcr/app/app.go` gets a new `AddCommand(delete.NewCmdAppDelete(f))` line.

## API Layer

Add `DeleteVonageApplication` to `pkg/api/deployment.go`:

```go
func (c *DeploymentClient) DeleteVonageApplication(ctx context.Context, appID string) error
// DELETE {baseURL}/applications/{appID}
// Returns nil on 2xx, ErrNotFound on 404, error on other non-2xx.
```

The `Factory` interface in `pkg/cmdutil/factory.go` already exposes `DeploymentClient()` — no interface changes needed.

## Output

| Scenario | Output | Exit code |
|---|---|---|
| Success | `Application "<id>" deleted.` to stdout | 0 |
| User aborts prompt | `Application removal aborted` to stderr | 0 |
| 404 | formatted error: `application not found` | 1 |
| Other API error | standard error via `pkg/format` | 1 |

## Tests

Table-driven tests in `delete_test.go` using `httpmock` and `testutil.NewTestIOStreams()`:

- Successful delete with `--yes` flag (no prompt).
- Successful delete after user confirms at prompt.
- User types `n` at prompt — aborts, no API call made.
- 404 response — error message printed, non-zero exit.
- 500 response — error message printed, non-zero exit.

## Consistency Notes

- Flag name `--yes` / `-y` matches `vcr instance remove` and `vcr secret remove`.
- Prompt text style matches `vcr instance remove`: lowercase, uses `opts.Survey().AskYesNo(...)`.
- Prompt is gated on `io.CanPrompt()` matching existing pattern.
- Error formatting uses `pkg/format` matching all other commands.
