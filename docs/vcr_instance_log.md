## vcr instance log

Fetch logs from a deployed VCR instance

### Synopsis

Fetch logs from a deployed VCR instance.

By default, the command retrieves the last N log entries (controlled by --history)
and exits. Use --follow (-f) to continuously stream new log entries until you
press Ctrl+C.

IDENTIFYING THE INSTANCE
  You can identify the instance using either:
  • --id: The unique instance UUID
  • --project-name + --instance-name: The combination from your manifest

LOG LEVELS
  Filter logs by severity level (shows specified level and above):
  • trace  - Most verbose, includes all logs
  • debug  - Debug information and above
  • info   - Informational messages and above
  • warn   - Warnings and above
  • error  - Errors and above
  • fatal  - Only fatal errors

SOURCE TYPES
  Filter logs by their source:
  • application  - Logs from your application code
  • provider     - Logs from VCR platform services

OUTPUT FORMAT
  Each log line shows: [timestamp] [source_type] message
  Example: 2024-01-15T10:30:00Z [application] Server started on port 3000


```
vcr instance log [flags]
```

### Examples

```
# Print the last logs by project and instance name (default, exits after output)
$ vcr instance log --project-name my-app --instance-name dev
2024-01-15T10:30:00Z [application] Server started on port 3000
2024-01-15T10:30:01Z [application] Connected to database

# Print the last logs by instance ID
$ vcr instance log --id 12345678-1234-1234-1234-123456789abc

# Continuously stream new logs (press Ctrl+C to stop)
$ vcr instance log -p my-app -n dev --follow
$ vcr instance log -p my-app -n dev -f

# Print the last 500 log entries and exit
$ vcr instance log -p my-app -n dev --history 500

# Filter to show only errors and above
$ vcr instance log -p my-app -n dev --log-level error

# Show only application logs (exclude provider logs)
$ vcr instance log -p my-app -n dev --source-type application

# Combine filters with follow
$ vcr instance log -p my-app -n dev -l warn -s application -f

```

### Options

```
  -f, --follow                 Continuously stream new log entries (press Ctrl+C to stop)
      --history int            Number of historical log entries to fetch initially (default: 300) (default 300)
  -i, --id string              Instance UUID (alternative to project-name + instance-name)
  -n, --instance-name string   Instance name (requires --project-name)
  -l, --log-level string       Minimum log level: trace, debug, info, warn, error, fatal
  -p, --project-name string    Project name (requires --instance-name)
  -s, --source-type string     Filter by source: application, provider
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

* [vcr instance](vcr_instance.md)	 - Manage deployed VCR instances
