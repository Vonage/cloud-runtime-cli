---
applyTo: "**/*.go"
---

# Copilot Instructions for Vonage Cloud Runtime CLI

## Priority Guidelines

When generating code for this repository:

1. **Factory Pattern First**: ALWAYS use the `cmdutil.Factory` interface for dependency injection - never instantiate API clients directly
2. **Go 1.24 Only**: Use only language features available in Go 1.24.0 - this is our version
3. **Error Wrapping**: Always wrap errors with context using `fmt.Errorf("context: %w", err)`
4. **Context Management**: Create deadline context from `opts.Deadline()` in command RunE functions
5. **Testing Required**: All code changes must include unit tests
6. **Consistent Code Style**: Match existing patterns exactly - scan similar files before generating new code

## Technology Stack

**Go Version**: 1.24.0

**Core Dependencies**:
- `github.com/spf13/cobra` v1.9.1 - CLI framework
- `github.com/go-resty/resty/v2` v2.16.5 - HTTP client
- `github.com/cli/cli/v2` v2.36.0 - IO streams and colors
- `github.com/AlecAivazis/survey/v2` v2.3.7 - Interactive prompts
- `github.com/golang/mock` v1.6.0 - Mocking (use mockgen)
- `gopkg.in/ini.v1` v1.67.0 - Config parsing
- `github.com/stretchr/testify` v1.10.0 - Test assertions

## Naming Conventions

### Files
- Lowercase with underscores: `create.go`, `create_test.go`
- Test files: Always `*_test.go` in same package
- Platform-specific: `command_gen_syscall_notwin.go`, `command_gen_syscall_win.go`

### Packages
- Lowercase, single-word: `api`, `config`, `cmdutil`, `format`
- Grouped by domain: `vcr/app/`, `vcr/deploy/`, `vcr/secret/`

### Types and Variables
- **Exported types**: `PascalCase` (e.g., `DeploymentClient`, `Options`)
- **Unexported types**: `camelCase` (e.g., `listRequest`, `createResponse`)
- **Interfaces**: End with `Interface` (e.g., `DeploymentInterface`, `AssetInterface`)
- **Functions**: Exported = `PascalCase`, Unexported = `camelCase`
- **Command factories**: `NewCmd<CommandName>` (e.g., `NewCmdDeploy`, `NewCmdAppCreate`)
- **Run functions**: `run<CommandName>` (e.g., `runDeploy`, `runCreate`)
- **Constants**: `PascalCase` (e.g., `DefaultTimeout`, `DefaultRegion`)

## Command Structure Pattern

Every CLI command MUST follow this exact structure:

```go
package commandname

import (
    "context"
    "fmt"
    "github.com/Vonage/vonage-cloud-runtime-cli/pkg/cmdutil"
    "github.com/spf13/cobra"
)

// Options holds the command-specific configuration
type Options struct {
    cmdutil.Factory  // ALWAYS embed the Factory interface

    // Command-specific fields matching flags
    Name        string
    SkipPrompts bool
}

// NewCmdCommandName creates the command
func NewCmdCommandName(f cmdutil.Factory) *cobra.Command {
    opts := Options{Factory: f}

    cmd := &cobra.Command{
        Use:   "commandname",
        Short: "Brief description",
        RunE: func(_ *cobra.Command, _ []string) error {
            // ALWAYS create context with deadline from Factory
            ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
            defer cancel()
            return runCommandName(ctx, &opts)
        },
    }

    // Define flags
    cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Resource name")

    return cmd
}

// runCommandName executes the command logic
func runCommandName(ctx context.Context, opts *Options) error {
    io := opts.IOStreams()
    c := io.ColorScheme()

    // Get clients from Factory (NEVER instantiate directly)
    deployClient := opts.DeploymentClient()

    // Use spinners for long operations
    spinner := cmdutil.DisplaySpinnerMessageWithHandle("Processing...")
    result, err := deployClient.SomeOperation(ctx, opts.Name)
    spinner.Stop()

    if err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }

    // Use color scheme for output
    fmt.Fprintf(io.Out, "%s Operation successful\n", c.SuccessIcon())
    fmt.Fprintf(io.Out, "%s ID: %s\n", c.Blue(cmdutil.InfoIcon), result.ID)

    return nil
}
```

## Testing Pattern

**All code changes require unit tests.** Table-driven tests are a common pattern in this codebase but not mandatory. Write tests that are clear and maintainable.

**Table-driven test example** (commonly used for commands with multiple scenarios):

```go
package commandname

import (
    "testing"
    "github.com/Vonage/vonage-cloud-runtime-cli/testutil"
    "github.com/golang/mock/gomock"
    "github.com/stretchr/testify/assert"
)

func TestCommandName(t *testing.T) {
    type mock struct {
        OperationTimes      int
        OperationReturnData interface{}
        OperationReturnErr  error
    }

    type want struct {
        errMsg string
        stdout string
    }

    tests := []struct {
        name string
        cli  string
        mock mock
        want want
    }{
        {
            name: "happy-path",
            cli:  "--name=Test",
            mock: mock{
                OperationTimes:      1,
                OperationReturnData: expectedData,
            },
            want: want{
                stdout: "Operation successful",
            },
        },
        {
            name: "error-case",
            cli:  "--name=Test",
            mock: mock{
                OperationTimes:     1,
                OperationReturnErr: errors.New("operation failed"),
            },
            want: want{
                errMsg: "operation failed",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()

            ios, _, stdout, _ := testutil.NewTestIOStreams()
            mockClient := mocks.NewMockDeploymentInterface(ctrl)

            if tt.mock.OperationTimes > 0 {
                mockClient.EXPECT().
                    SomeOperation(gomock.Any(), gomock.Any()).
                    Return(tt.mock.OperationReturnData, tt.mock.OperationReturnErr).
                    Times(tt.mock.OperationTimes)
            }

            f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, mockClient, nil, nil)

            cmd := NewCmdCommandName(f)
            cmd.SetArgs(strings.Split(tt.cli, " "))
            cmd.SetOut(ios.Out)
            cmd.SetErr(ios.ErrOut)

            err := cmd.Execute()

            if tt.want.errMsg != "" {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.want.errMsg)
            } else {
                assert.NoError(t, err)
            }

            if tt.want.stdout != "" {
                assert.Contains(t, stdout.String(), tt.want.stdout)
            }
        })
    }
}
```

**Simple unit test example** (for straightforward functions):

```go
func TestParseConfig(t *testing.T) {
    cfg, err := ParseConfig("testdata/config.yaml")
    assert.NoError(t, err)
    assert.Equal(t, "expected-value", cfg.Field)
}
```

## Error Handling Pattern

Use our custom error types and ALWAYS wrap with context:

```go
// For API errors - wrap with context
if err != nil {
    return fmt.Errorf("failed to create application: %w", err)
}

// Check for specific error types
if errors.Is(err, cmdutil.ErrCancel) {
    return nil  // User cancelled
}

// Type assertions for API errors
var apiErr api.Error
if errors.As(err, &apiErr) {
    // Access TraceID, ContainerLogs, etc.
    fmt.Fprintf(io.ErrOut, "Trace ID: %s\n", apiErr.TraceID)
}

// Mutual exclusivity checking
if err := cmdutil.MutuallyExclusive("cannot use both flags", flagA != "", flagB != ""); err != nil {
    return err
}
```

## API Client Pattern

When creating new API clients, follow this exact structure:

```go
package api

import (
    "context"
    "github.com/go-resty/resty/v2"
)

// Client struct with baseURL and injected httpClient
type ResourceClient struct {
    baseURL    string
    httpClient *resty.Client
}

// Constructor receives URL and client (NEVER create client here)
func NewResourceClient(baseURL string, httpClient *resty.Client) *ResourceClient {
    return &ResourceClient{
        baseURL:    baseURL,
        httpClient: httpClient,
    }
}

// Request/response types (unexported for internal API types)
type createRequest struct {
    Name string `json:"name"`
}

type createResponse struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

// Methods with context, check IsError() before processing
func (c *ResourceClient) CreateResource(ctx context.Context, name string) (*createResponse, error) {
    resp, err := c.httpClient.R().
        SetContext(ctx).
        SetResult(&createResponse{}).
        SetBody(createRequest{Name: name}).
        Post(c.baseURL + "/create")

    if err != nil {
        return nil, err
    }

    if resp.IsError() {
        return nil, NewErrorFromHTTPResponse(resp)
    }

    result := resp.Result().(*createResponse)
    return result, nil
}
```

## Configuration Management

Follow this precedence order (highest to lowest):
1. Command-line flags (highest priority)
2. Deployment manifest (`vcr.yaml`)
3. Home config file (`~/.vcr-cli`) (lowest priority)

Access config through Factory methods:
- `opts.APIKey()` - Get API key (resolved from flags/config)
- `opts.Region()` - Get region
- `opts.Deadline()` - Get deadline for context

## Output Formatting

Use IOStreams ColorScheme for consistent output:

```go
io := opts.IOStreams()
c := io.ColorScheme()

// Success messages
fmt.Fprintf(io.Out, "%s Operation completed\n", c.SuccessIcon())  // ✓

// Info messages
fmt.Fprintf(io.Out, "%s ID: %s\n", c.Blue(cmdutil.InfoIcon), id)  // ℹ

// Warnings
fmt.Fprintf(io.Out, "%s Warning: xyz\n", c.WarningIcon())  // ⚠

// Errors (to stderr)
fmt.Fprintf(io.ErrOut, "%s Operation failed\n", c.FailureIcon())  // ✗

// Spinners for long operations
spinner := cmdutil.DisplaySpinnerMessageWithHandle("Processing...")
// ... do work ...
spinner.Stop()
```

## Project Structure

```
.
├── main.go                    # Entry point, error formatting
├── Makefile                   # build, test, run targets
├── pkg/
│   ├── api/                   # API clients (Asset, Deployment, Release, Datastore)
│   ├── cmdutil/               # Factory, error types, utilities
│   ├── config/                # Config/manifest parsing (INI, YAML)
│   └── format/                # Output formatting, tables
├── vcr/
│   ├── root/                  # Root command, global flags
│   ├── app/                   # Application commands (create, list, generatekeys)
│   ├── deploy/                # Deployment command
│   ├── configure/             # Configuration command
│   ├── secret/                # Secret management
│   ├── mongo/                 # MongoDB operations
│   └── instance/              # Instance management
├── testutil/                  # Test helpers, mock factory
│   └── mocks/                 # Generated mocks (mockgen)
└── tests/integration/         # Integration tests with Docker
```

## Code Generation

Use `//go:generate` directives for mock generation:

```go
//go:generate mockgen -source=factory.go -destination=../../testutil/mocks/mock_factory.go -package=mocks
```

Run with: `go generate ./...`

## Common Patterns to Avoid

❌ **DON'T** instantiate API clients directly:
```go
client := api.NewDeploymentClient(url, httpClient)  // WRONG
```

✅ **DO** use Factory interface:
```go
client := opts.DeploymentClient()  // CORRECT
```

❌ **DON'T** ignore context:
```go
result, err := client.Operation(name)  // WRONG - missing context
```

✅ **DO** pass context from Factory deadline:
```go
ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
defer cancel()
result, err := client.Operation(ctx, name)  // CORRECT
```

❌ **DON'T** create code without tests:
```go
// New feature with no test file  // WRONG
```

✅ **DO** include unit tests for all changes:
```go
// create.go + create_test.go  // CORRECT
```

## Additional Notes

- **All changes require tests** - No exceptions
- **Use heredoc for multi-line strings**: `heredoc.Doc(`...`)`
- **Platform-specific code**: Use build tags and separate files (`_win.go`, `_notwin.go`)
- **Linting**: Code must pass `golangci-lint` (see `.golangci.yml`)
- **Dependencies**: Update via `make deps`, never commit `vendor/`

---

For examples, see existing commands in `vcr/app/create/`, `vcr/deploy/`, etc.
For more details, see `README.md`, `PLAN.md`, and command docs in `docs/`.
