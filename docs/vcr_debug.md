## vcr debug

Run your application locally in debug mode with live VCR integration

### Synopsis

Run your application locally in debug mode with live VCR integration.

Debug mode allows you to run your application code locally while connected to the
VCR platform. A remote debug server acts as a proxy, forwarding requests from Vonage
services (webhooks, events) to your local machine.

HOW IT WORKS
  1. A debug proxy server is deployed to VCR
  2. Your local application starts and connects to the proxy
  3. Vonage webhooks/events are forwarded to your local machine
  4. You can set breakpoints and debug your code in real-time

REQUIREMENTS
  • A vcr.yml manifest with a debug.entrypoint defined
  • A Vonage application linked to your project
  • The debug.application-id can differ from instance.application-id

VS CODE DEBUGGER INTEGRATION
  To attach VS Code debugger, add this configuration to .vscode/launch.json:

  {
    "version": "0.2.0",
    "configurations": [
      {
        "type": "node",
        "request": "attach",
        "name": "Attach VCR Debugger",
        "port": 9229,
        "restart": true,
        "localRoot": "${workspaceFolder}"
      }
    ]
  }

ENVIRONMENT VARIABLES
  Environment variables from debug.environment (or instance.environment as fallback)
  in your manifest are loaded. For secrets, export them locally before running debug.

CLEANUP
  Press Ctrl+C to stop debug mode. The remote debug server is automatically removed
  unless --preserve-data is specified.


```
vcr debug [path_to_project] [flags]
```

### Examples

```
# Start debug mode in the current directory
$ vcr debug

# Start debug mode in a specific directory
$ vcr debug ./my-project

# Use a custom name for the debug proxy (makes URL deterministic)
$ vcr debug --name my-debugger

# Use a specific Vonage application for debugging
$ vcr debug --app-id 12345678-1234-1234-1234-123456789abc

# Change the local application port (default: 3000)
$ vcr debug --app-port 8080

# Change the debugger proxy port (default: 3001)
$ vcr debug --debugger-port 4000

# Preserve data after debug session ends
$ vcr debug --preserve-data

# Skip confirmation prompts
$ vcr debug --yes

# Enable verbose logging for troubleshooting
$ vcr debug --verbose

# Use a specific manifest file
$ vcr debug --filename ./custom-vcr.yml

```

### Options

```
  -i, --app-id string       Vonage application ID for debugging (overrides manifest)
  -a, --app-port int        Local port your application listens on (default: 3000) (default 3000)
  -d, --debugger-port int   Local port for debugger proxy server (default: 3001) (default 3001)
  -f, --filename string     Path to VCR manifest file (default: vcr.yml in project directory)
  -n, --name string         Name for the debug proxy server (creates deterministic URL)
      --preserve-data       Keep debug session data after stopping (useful for debugging state issues)
  -r, --runtime string      Runtime environment (overrides manifest, e.g., nodejs18)
  -v, --verbose             Enable verbose logging for troubleshooting
  -y, --yes                 Skip confirmation prompts
```

### Options inherited from parent commands

```
      --api-key string            Vonage API key
      --api-secret string         Vonage API secret
      --config-file string        Path to config file (default is $HOME/.vcr-cli) (default "~/.vcr-cli")
      --endpoint string           Platform API base URL (overrides --graphql-endpoint and the config file)
      --graphql-endpoint string   Deprecated: use --endpoint. GraphQL endpoint used to fetch metadata
      --help                      Show help for command
      --region string             Vonage platform region
  -t, --timeout duration          Overall deadline for the command, including waiting for a build to finish (e.g. 20m) (default 10m0s)
```

### SEE ALSO

* [vcr](vcr.md)	 - Streamline your Vonage Cloud Runtime development and management tasks with VCR
* [vcr debug prune-sessions](vcr_debug_prune-sessions.md)	 - Remove all active debug sessions
