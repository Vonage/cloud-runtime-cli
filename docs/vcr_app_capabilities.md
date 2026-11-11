## vcr app capabilities

Manage the application-capabilities block in vcr.yml

### Synopsis

Manage the application-capabilities block in vcr.yml.

The application-capabilities block of the manifest configures the capabilities
of the Vonage application linked to your deployment, with webhook addresses
that can use {{instance_url}}. These commands edit only that block and work
offline: they need no configuration, credentials or network.

AVAILABLE COMMANDS
  add  Add default capabilities to the application-capabilities block


### Examples

```
# Add the default Voice and Messages capabilities to vcr.yml
$ vcr app capabilities add --voice --messages

# Print the block instead of writing it
$ vcr app capabilities add --rtc --print

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
* [vcr app capabilities add](vcr_app_capabilities_add.md)	 - Add default capabilities to the application-capabilities block
