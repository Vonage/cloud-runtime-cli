## vcr app create

Create a new Vonage application

### Synopsis

Create a new Vonage application for use with VCR.

This command creates a Vonage application with the specified capabilities enabled.
The application ID returned can be used in your vcr.yml manifest file to link
your VCR deployment with the Vonage platform.

CAPABILITIES
  Applications can have one or more capabilities enabled:
  • Voice     (-v, --voice)    - Enable Voice API for phone calls
  • Messages  (-m, --messages) - Enable Messages API for SMS, WhatsApp, etc.
  • RTC       (-r, --rtc)      - Enable Real-Time Communication for in-app voice/video

NOTE: If no capabilities are specified, the application is created without any
enabled capabilities. You can enable capabilities later via the Vonage Dashboard.


```
vcr app create [flags]
```

### Examples

```
# Create a basic application
$ vcr app create --name my-app
✓ Application created
ℹ id: 12345678-1234-1234-1234-123456789abc
ℹ name: my-app

# Create an application with Voice capability enabled
$ vcr app create --name voice-app --voice

# Create an application with multiple capabilities
$ vcr app create --name full-app --voice --messages --rtc

# Create without confirmation prompt
$ vcr app create --name my-app --yes

```

### Options

```
  -m, --messages      Enable Messages API capability
  -n, --name string   Name of the application (required)
  -r, --rtc           Enable RTC (Real-Time Communication) capability
  -v, --voice         Enable Voice API capability
  -y, --yes           Skip confirmation prompts
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

* [vcr app](vcr_app.md)	 - Manage Vonage applications for VCR deployments
