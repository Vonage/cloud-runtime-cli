# vcr app delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `vcr app delete <applicationID> [--yes|-y]` command that deletes a Vonage application via the deployment API with an interactive confirmation prompt.

**Architecture:** Add `DeleteVonageApplication` to the `DeploymentClient` and `DeploymentInterface`, then implement the command under `vcr/app/delete/` following the same pattern as `vcr/instance/remove/`. Register the new subcommand in `vcr/app/app.go`. Regenerate mocks after the interface change.

**Tech Stack:** Go, Cobra, resty, gomock, testify, httpmock

---

## File Map

| Action | File |
|--------|------|
| Modify | `pkg/api/deployment.go` — add `DeleteVonageApplication` method |
| Modify | `pkg/cmdutil/factory.go` — add `DeleteVonageApplication` to `DeploymentInterface` |
| Regenerate | `testutil/mocks/factory.go` — run `go generate ./...` |
| Create | `vcr/app/delete/delete.go` — command implementation |
| Create | `vcr/app/delete/delete_test.go` — table-driven tests |
| Modify | `vcr/app/app.go` — register `deleteCmd.NewCmdAppDelete(f)` |

---

## Task 1: Add `DeleteVonageApplication` to the API client

**Files:**
- Modify: `pkg/api/deployment.go`

- [ ] **Step 1: Add the method after `GenerateVonageApplicationKeys` (line 107)**

  Open `pkg/api/deployment.go` and add the following method after the closing brace of `GenerateVonageApplicationKeys` (after line 107):

  ```go
  func (c *DeploymentClient) DeleteVonageApplication(ctx context.Context, appID string) error {
  	resp, err := c.httpClient.R().
  		SetContext(ctx).
  		Delete(fmt.Sprintf("%s/applications/%s", c.baseURL, appID))
  	if err != nil {
  		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
  	}
  	if resp.StatusCode() == http.StatusNotFound {
  		return ErrNotFound
  	}
  	if resp.IsError() {
  		return NewErrorFromHTTPResponse(resp)
  	}
  	return nil
  }
  ```

  Note: `http` is already imported. `ErrNotFound` is defined in `pkg/api/error.go`.

- [ ] **Step 2: Verify it compiles**

  ```bash
  go build ./pkg/api/...
  ```

  Expected: no output (clean build).

- [ ] **Step 3: Commit**

  ```bash
  git add pkg/api/deployment.go
  git commit -m "feat(api): add DeleteVonageApplication to DeploymentClient [APIAPEX-2823]"
  ```

---

## Task 2: Add `DeleteVonageApplication` to `DeploymentInterface` and regenerate mocks

**Files:**
- Modify: `pkg/cmdutil/factory.go`
- Regenerate: `testutil/mocks/factory.go`

- [ ] **Step 1: Add the method signature to `DeploymentInterface`**

  In `pkg/cmdutil/factory.go`, inside the `DeploymentInterface` block (lines 39-62), add after line 42 (`GenerateVonageApplicationKeys`):

  ```go
  DeleteVonageApplication(ctx context.Context, appID string) error
  ```

  The interface block should now read:

  ```go
  type DeploymentInterface interface {
  	CreateVonageApplication(ctx context.Context, name string, enableRTC, enableVoice, enableMessages bool) (api.CreateVonageApplicationOutput, error)
  	ListVonageApplications(ctx context.Context, filter string) (api.ListVonageApplicationsOutput, error)
  	GenerateVonageApplicationKeys(ctx context.Context, appID string) error
  	DeleteVonageApplication(ctx context.Context, appID string) error
  	// ... rest unchanged
  ```

- [ ] **Step 2: Regenerate mocks**

  ```bash
  go generate ./...
  ```

  Expected: `testutil/mocks/factory.go` is updated with a new `DeleteVonageApplication` mock method. No errors.

- [ ] **Step 3: Verify it compiles**

  ```bash
  go build ./...
  ```

  Expected: no output (clean build).

- [ ] **Step 4: Commit**

  ```bash
  git add pkg/cmdutil/factory.go testutil/mocks/factory.go
  git commit -m "feat(cmdutil): add DeleteVonageApplication to DeploymentInterface [APIAPEX-2823]"
  ```

---

## Task 3: Implement `vcr/app/delete/delete.go`

**Files:**
- Create: `vcr/app/delete/delete.go`

- [ ] **Step 1: Create the file**

  Create `vcr/app/delete/delete.go` with the following content:

  ```go
  package delete

  import (
  	"context"
  	"errors"
  	"fmt"

  	"github.com/MakeNowJust/heredoc"
  	"github.com/spf13/cobra"

  	"vonage-cloud-runtime-cli/pkg/api"
  	"vonage-cloud-runtime-cli/pkg/cmdutil"
  )

  type Options struct {
  	cmdutil.Factory

  	ApplicationID string
  	SkipPrompts   bool
  }

  func NewCmdAppDelete(f cmdutil.Factory) *cobra.Command {
  	opts := Options{
  		Factory: f,
  	}

  	cmd := &cobra.Command{
  		Use:   "delete <applicationID>",
  		Short: "Delete a Vonage application",
  		Long: heredoc.Doc(`Delete a Vonage application from your account.

  			This command permanently deletes a Vonage application and its associated
  			credentials. Any VCR instances linked to this application will lose their
  			authentication credentials on next restart.

  			WARNING: This action is irreversible. Make sure no running instances
  			depend on this application before deleting it.
  		`),
  		Example: heredoc.Doc(`
  			# Delete an application (will prompt for confirmation)
  			$ vcr app delete 12345678-1234-1234-1234-123456789abc

  			# Delete without confirmation prompt (useful for CI/CD)
  			$ vcr app delete 12345678-1234-1234-1234-123456789abc --yes
  		`),
  		Args: cobra.ExactArgs(1),
  		RunE: func(_ *cobra.Command, args []string) error {
  			opts.ApplicationID = args[0]

  			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
  			defer cancel()

  			return runDelete(ctx, &opts)
  		},
  	}

  	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompt (use with caution)")

  	return cmd
  }

  func runDelete(ctx context.Context, opts *Options) error {
  	io := opts.IOStreams()
  	c := io.ColorScheme()

  	if io.CanPrompt() && !opts.SkipPrompts {
  		if !opts.Survey().AskYesNo(fmt.Sprintf("are you sure you want to delete application %q ?", opts.ApplicationID)) {
  			fmt.Fprintf(io.ErrOut, "%s Application removal aborted\n", c.WarningIcon())
  			return nil
  		}
  	}

  	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Deleting application %q...", opts.ApplicationID))
  	err := opts.DeploymentClient().DeleteVonageApplication(ctx, opts.ApplicationID)
  	spinner.Stop()
  	if err != nil {
  		if errors.Is(err, api.ErrNotFound) {
  			return fmt.Errorf("application %q not found", opts.ApplicationID)
  		}
  		return fmt.Errorf("failed to delete application: %w", err)
  	}

  	fmt.Fprintf(io.Out, "%s Application %q successfully deleted\n", c.SuccessIcon(), opts.ApplicationID)

  	return nil
  }
  ```

- [ ] **Step 2: Verify it compiles**

  ```bash
  go build ./vcr/app/delete/...
  ```

  Expected: no output.

- [ ] **Step 3: Commit**

  ```bash
  git add vcr/app/delete/delete.go
  git commit -m "feat(app): implement vcr app delete command [APIAPEX-2823]"
  ```

---

## Task 4: Write tests for `vcr/app/delete/delete_test.go`

**Files:**
- Create: `vcr/app/delete/delete_test.go`

- [ ] **Step 1: Create the test file**

  Create `vcr/app/delete/delete_test.go` with the following content:

  ```go
  package delete

  import (
  	"bytes"
  	"errors"
  	"io"
  	"testing"

  	"github.com/cli/cli/v2/pkg/iostreams"
  	"github.com/golang/mock/gomock"
  	"github.com/google/shlex"
  	"github.com/stretchr/testify/require"

  	"vonage-cloud-runtime-cli/pkg/api"
  	"vonage-cloud-runtime-cli/testutil"
  	"vonage-cloud-runtime-cli/testutil/mocks"
  )

  func TestAppDelete(t *testing.T) {
  	const appID = "12345678-1234-1234-1234-123456789abc"

  	type mock struct {
  		DeleteTimes     int
  		DeleteReturnErr error
  		AskYesNoTimes   int
  		AskYesNoReturn  bool
  	}
  	type want struct {
  		errMsg string
  		stdout string
  		stderr string
  	}

  	tests := []struct {
  		name string
  		cli  string
  		mock mock
  		want want
  	}{
  		{
  			name: "happy-path-with-yes-flag",
  			cli:  appID + " --yes",
  			mock: mock{
  				DeleteTimes:     1,
  				DeleteReturnErr: nil,
  				AskYesNoTimes:   0,
  			},
  			want: want{
  				stdout: "✓ Application \"" + appID + "\" successfully deleted\n",
  			},
  		},
  		{
  			name: "happy-path-confirm-prompt",
  			cli:  appID,
  			mock: mock{
  				DeleteTimes:     1,
  				DeleteReturnErr: nil,
  				AskYesNoTimes:   1,
  				AskYesNoReturn:  true,
  			},
  			want: want{
  				stdout: "✓ Application \"" + appID + "\" successfully deleted\n",
  			},
  		},
  		{
  			name: "user-aborts-prompt",
  			cli:  appID,
  			mock: mock{
  				DeleteTimes:     0,
  				AskYesNoTimes:   1,
  				AskYesNoReturn:  false,
  			},
  			want: want{
  				stderr: "! Application removal aborted\n",
  			},
  		},
  		{
  			name: "missing-application-id",
  			cli:  "",
  			mock: mock{
  				DeleteTimes:   0,
  				AskYesNoTimes: 0,
  			},
  			want: want{
  				errMsg: "accepts 1 arg(s), received 0",
  			},
  		},
  		{
  			name: "not-found",
  			cli:  appID + " --yes",
  			mock: mock{
  				DeleteTimes:     1,
  				DeleteReturnErr: api.ErrNotFound,
  			},
  			want: want{
  				errMsg: "application \"" + appID + "\" not found",
  			},
  		},
  		{
  			name: "api-error",
  			cli:  appID + " --yes",
  			mock: mock{
  				DeleteTimes:     1,
  				DeleteReturnErr: errors.New("internal server error"),
  			},
  			want: want{
  				errMsg: "failed to delete application: internal server error",
  			},
  		},
  	}

  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			ctrl := gomock.NewController(t)

  			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
  			deploymentMock.EXPECT().
  				DeleteVonageApplication(gomock.Any(), appID).
  				Times(tt.mock.DeleteTimes).
  				Return(tt.mock.DeleteReturnErr)

  			surveyMock := mocks.NewMockSurveyInterface(ctrl)
  			surveyMock.EXPECT().
  				AskYesNo(gomock.Any()).
  				Times(tt.mock.AskYesNoTimes).
  				Return(tt.mock.AskYesNoReturn)

  			ios, _, stdout, stderr := iostreams.Test()
  			ios.SetStdinTTY(true)
  			ios.SetStdoutTTY(true)

  			argv, err := shlex.Split(tt.cli)
  			if err != nil {
  				t.Fatal(err)
  			}

  			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, surveyMock, nil)

  			cmd := NewCmdAppDelete(f)
  			cmd.SetArgs(argv)
  			cmd.SetIn(&bytes.Buffer{})
  			cmd.SetOut(io.Discard)
  			cmd.SetErr(io.Discard)

  			if _, err := cmd.ExecuteC(); err != nil && tt.want.errMsg != "" {
  				require.Error(t, err, "should throw error")
  				require.Equal(t, tt.want.errMsg, err.Error())
  				return
  			}

  			cmdOut := &testutil.CmdOut{
  				OutBuf: stdout,
  				ErrBuf: stderr,
  			}

  			if tt.want.stderr != "" {
  				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
  				return
  			}
  			require.NoError(t, err, "should not throw error")
  			require.Equal(t, tt.want.stdout, cmdOut.String())
  		})
  	}
  }
  ```

- [ ] **Step 2: Run the tests**

  ```bash
  go test -v ./vcr/app/delete/...
  ```

  Expected: all 6 test cases PASS.

- [ ] **Step 3: Commit**

  ```bash
  git add vcr/app/delete/delete_test.go
  git commit -m "test(app): add tests for vcr app delete command [APIAPEX-2823]"
  ```

---

## Task 5: Register the delete subcommand in `vcr/app/app.go`

**Files:**
- Modify: `vcr/app/app.go`

- [ ] **Step 1: Add the import and `AddCommand` call**

  In `vcr/app/app.go`, add the import:

  ```go
  deleteCmd "vonage-cloud-runtime-cli/vcr/app/delete"
  ```

  And add the `AddCommand` call before `return cmd`:

  ```go
  cmd.AddCommand(deleteCmd.NewCmdAppDelete(f))
  ```

  The full updated imports block:

  ```go
  import (
  	"github.com/MakeNowJust/heredoc"
  	"github.com/spf13/cobra"

  	"vonage-cloud-runtime-cli/pkg/cmdutil"
  	createCmd "vonage-cloud-runtime-cli/vcr/app/create"
  	deleteCmd "vonage-cloud-runtime-cli/vcr/app/delete"
  	generatekeysCmd "vonage-cloud-runtime-cli/vcr/app/generatekeys"
  	listCmd "vonage-cloud-runtime-cli/vcr/app/list"
  )
  ```

  And the updated `AddCommand` calls:

  ```go
  cmd.AddCommand(listCmd.NewCmdAppList(f))
  cmd.AddCommand(createCmd.NewCmdAppCreate(f))
  cmd.AddCommand(generatekeysCmd.NewCmdAppGenerateKeys(f))
  cmd.AddCommand(deleteCmd.NewCmdAppDelete(f))
  ```

  Also update the `AVAILABLE COMMANDS` section in the Long description to include `delete`:

  ```
  AVAILABLE COMMANDS
    create         Create a new Vonage application
    list (ls)      List all Vonage applications in your account
    generate-keys  Generate new key pairs for an existing application
    delete         Delete a Vonage application
  ```

- [ ] **Step 2: Run the full test suite**

  ```bash
  go test ./...
  ```

  Expected: all tests PASS, no failures.

- [ ] **Step 3: Build and do a quick smoke test**

  ```bash
  make build
  ./vcr app delete --help
  ```

  Expected: help text shows `delete <applicationID>` with `--yes` / `-y` flag documented.

- [ ] **Step 4: Commit**

  ```bash
  git add vcr/app/app.go
  git commit -m "feat(app): register vcr app delete subcommand [APIAPEX-2823]"
  ```
