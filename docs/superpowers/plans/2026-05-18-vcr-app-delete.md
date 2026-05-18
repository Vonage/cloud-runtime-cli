# vcr app remove Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `vcr app remove <applicationID> [--yes|-y]` command that removes a Vonage application via the deployment API with an interactive confirmation prompt.

**Architecture:** Add `DeleteVonageApplication` to the `DeploymentClient` and `DeploymentInterface`, then implement the command under `vcr/app/remove/` following the same pattern as `vcr/instance/remove/`. Register the new subcommand in `vcr/app/app.go`. Regenerate mocks after the interface change.

**Tech Stack:** Go, Cobra, resty, gomock, testify

---

## File Map

| Action | File |
|--------|------|
| Modify | `pkg/api/deployment.go` — add `DeleteVonageApplication` method |
| Modify | `pkg/cmdutil/factory.go` — add `DeleteVonageApplication` to `DeploymentInterface` |
| Regenerate | `testutil/mocks/factory.go` — run `go generate ./...` |
| Create | `vcr/app/remove/remove.go` — command implementation |
| Create | `vcr/app/remove/remove_test.go` — table-driven tests |
| Modify | `vcr/app/app.go` — register `removeCmd.NewCmdAppRemove(f)` |

---

## Task 1: Add `DeleteVonageApplication` to the API client

**Files:**
- Modify: `pkg/api/deployment.go`

- [x] **Step 1: Add the method after `GenerateVonageApplicationKeys`**

  ```go
  func (c *DeploymentClient) DeleteVonageApplication(ctx context.Context, appID string) error {
  	resp, err := c.httpClient.R().
  		SetContext(ctx).
  		Delete(c.baseURL + "/applications/" + appID)
  	if err != nil {
  		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
  	}
  	if resp.StatusCode() == http.StatusNotFound {
  		return nil
  	}
  	if resp.IsError() {
  		return NewErrorFromHTTPResponse(resp)
  	}
  	return nil
  }
  ```

  Note: returns `nil` on 404 (idempotent). Uses string concatenation consistent with adjacent methods.

- [x] **Step 2: Verify it compiles**

  ```bash
  go build ./pkg/api/...
  ```

- [x] **Step 3: Commit**

  ```bash
  git commit -m "feat(api): add DeleteVonageApplication to DeploymentClient [APIAPEX-2823]"
  ```

---

## Task 2: Add `DeleteVonageApplication` to `DeploymentInterface` and regenerate mocks

**Files:**
- Modify: `pkg/cmdutil/factory.go`
- Regenerate: `testutil/mocks/factory.go`

- [x] **Step 1: Add the method signature to `DeploymentInterface`** after `GenerateVonageApplicationKeys`:

  ```go
  DeleteVonageApplication(ctx context.Context, appID string) error
  ```

- [x] **Step 2: Regenerate mocks**

  ```bash
  go generate ./...
  ```

- [x] **Step 3: Verify it compiles**

  ```bash
  go build ./...
  ```

- [x] **Step 4: Commit**

  ```bash
  git commit -m "feat(cmdutil): add DeleteVonageApplication to DeploymentInterface [APIAPEX-2823]"
  ```

---

## Task 3: Implement `vcr/app/remove/remove.go`

**Files:**
- Create: `vcr/app/remove/remove.go`

- [x] **Step 1: Create the file**

  ```go
  package remove

  import (
  	"context"
  	"fmt"

  	"github.com/MakeNowJust/heredoc"
  	"github.com/spf13/cobra"

  	"vonage-cloud-runtime-cli/pkg/cmdutil"
  )

  type Options struct {
  	cmdutil.Factory

  	ApplicationID string
  	SkipPrompts   bool
  }

  func NewCmdAppRemove(f cmdutil.Factory) *cobra.Command {
  	opts := Options{Factory: f}

  	cmd := &cobra.Command{
  		Use:     "remove <applicationID>",
  		Aliases: []string{"rm"},
  		Short:   "Remove a Vonage application",
  		// ...
  		Args: cobra.ExactArgs(1),
  		RunE: func(_ *cobra.Command, args []string) error {
  			opts.ApplicationID = args[0]
  			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
  			defer cancel()
  			return runRemove(ctx, &opts)
  		},
  	}

  	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompt (use with caution)")
  	return cmd
  }

  func runRemove(ctx context.Context, opts *Options) error {
  	io := opts.IOStreams()
  	c := io.ColorScheme()

  	if io.CanPrompt() && !opts.SkipPrompts {
  		if !opts.Survey().AskYesNo(fmt.Sprintf("Are you sure you want to remove application %q?", opts.ApplicationID)) {
  			fmt.Fprintf(io.ErrOut, "%s Application removal aborted\n", c.WarningIcon())
  			return nil
  		}
  	}

  	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Removing application %q...", opts.ApplicationID))
  	err := opts.DeploymentClient().DeleteVonageApplication(ctx, opts.ApplicationID)
  	spinner.Stop()
  	if err != nil {
  		return fmt.Errorf("failed to remove application: %w", err)
  	}

  	fmt.Fprintf(io.Out, "%s Application %q successfully removed\n", c.SuccessIcon(), opts.ApplicationID)
  	return nil
  }
  ```

- [x] **Step 2: Verify it compiles**

  ```bash
  go build ./vcr/app/remove/...
  ```

- [x] **Step 3: Commit**

  ```bash
  git commit -m "feat(app): implement vcr app remove command [APIAPEX-2823]"
  ```

---

## Task 4: Write tests for `vcr/app/remove/remove_test.go`

**Files:**
- Create: `vcr/app/remove/remove_test.go`

- [x] **Step 1: Create the file with 5 test cases**

  | Test case | DeleteTimes | AskYesNoTimes | Expected |
  |---|---|---|---|
  | `happy-path-with-yes-flag` | 1 | 0 | stdout: success message |
  | `happy-path-confirm-prompt` | 1 | 1 (returns true) | stdout: success message |
  | `user-aborts-prompt` | 0 | 1 (returns false) | stderr: abort message |
  | `missing-application-id` | 0 | 0 | errMsg: cobra arg error |
  | `api-error` | 1 | 0 | errMsg: wrapped error |

  Note: no `not-found` case — `DeleteVonageApplication` returns `nil` on 404.

- [x] **Step 2: Run the tests**

  ```bash
  go test -v ./vcr/app/remove/...
  ```

  Expected: all 5 PASS.

- [x] **Step 3: Commit**

  ```bash
  git commit -m "test(app): add tests for vcr app remove command [APIAPEX-2823]"
  ```

---

## Task 5: Register the remove subcommand in `vcr/app/app.go`

**Files:**
- Modify: `vcr/app/app.go`

- [x] **Step 1: Add import and `AddCommand` call**

  ```go
  removeCmd "vonage-cloud-runtime-cli/vcr/app/remove"
  ```

  ```go
  cmd.AddCommand(removeCmd.NewCmdAppRemove(f))
  ```

  Update `AVAILABLE COMMANDS` in Long description to include `remove (rm)`.

- [x] **Step 2: Run the full test suite**

  ```bash
  go test ./...
  ```

- [x] **Step 3: Smoke test**

  ```bash
  make build && ./vcr app remove --help
  ```

- [x] **Step 4: Commit**

  ```bash
  git commit -m "feat(app): register vcr app remove subcommand [APIAPEX-2823]"
  ```
