## vcr app

Manage Vonage applications for VCR deployments

### Synopsis

Manage Vonage applications for VCR deployments.

Vonage applications are containers that hold your communication settings and
capabilities. Each VCR deployment must be linked to a Vonage application.

WHAT IS A VONAGE APPLICATION?
  A Vonage application provides:
  • Authentication credentials (API keys and private keys)
  • Enabled capabilities (Voice, Messages, RTC)
  • Webhook URLs for receiving events

AVAILABLE COMMANDS
  create         Create a new Vonage application
  remove (rm)    Remove a Vonage application
  list (ls)      List all Vonage applications in your account
  generate-keys  Generate new key pairs for an existing application

WORKFLOW
  1. Create an application: vcr app create --name my-app
  2. Use the application ID in your vcr.yml manifest
  3. Deploy your VCR application with: vcr deploy


### Examples

```
# Create a new application with Voice and Messages capabilities
$ vcr app create --name my-app --voice --messages

# Remove an application
$ vcr app remove 12345678-1234-1234-1234-123456789abc

# Generate new keys for an existing application
$ vcr app generate-keys --app-id 12345678-1234-1234-1234-123456789abc

# List all applications
$ vcr app list

# List applications filtered by name
$ vcr app list --filter "production"

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
* [vcr app create](vcr_app_create.md)	 - Create a new Vonage application
* [vcr app generate-keys](vcr_app_generate-keys.md)	 - Generate new key pairs for a Vonage application
* [vcr app list](vcr_app_list.md)	 - List all Vonage applications in your account
* [vcr app remove](vcr_app_remove.md)	 - Remove a Vonage application
