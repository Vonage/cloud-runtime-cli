## vcr app generate-keys

Generate new key pairs for a Vonage application

### Synopsis

Generate new public/private key pairs for a Vonage application.

This command regenerates the authentication keys for a Vonage application,
allowing the VCR platform to access the application's credentials.

WHEN TO USE THIS COMMAND
  • You created an application via the Vonage Dashboard (not the CLI)
  • You need to rotate your application's keys for security
  • You're troubleshooting authentication issues with VCR

WARNING: Regenerating keys will invalidate any existing private keys for this
application. Any services using the old keys will need to be updated.

FINDING YOUR APPLICATION ID
  Use 'vcr app list' to see all your applications and their IDs.


```
vcr app generate-keys --app-id <application-id> [flags]
```

### Examples

```
# Generate new keys for an application
$ vcr app generate-keys --app-id 42066b10-c4ae-48a0-addd-feb2bd615a67
✓ Application "42066b10-c4ae-48a0-addd-feb2bd615a67" configured with newly generated keys

# Using the short flag
$ vcr app generate-keys -i 42066b10-c4ae-48a0-addd-feb2bd615a67

```

### Options

```
  -i, --app-id string   The UUID of the Vonage application (required)
```

### Options inherited from parent commands

```
      --api-key string            Vonage API key
      --api-secret string         Vonage API secret
      --config-file string        Path to config file (default is $HOME/.vcr-cli) (default "~/.vcr-cli")
      --graphql-endpoint string   Graphql endpoint used to fetch metadata
      --help                      Show help for command
      --region string             Vonage platform region
  -t, --timeout duration          Overall deadline for the command, including waiting for a build to finish (e.g. 20m) (default 10m0s)
```

### SEE ALSO

* [vcr app](vcr_app.md)	 - Manage Vonage applications for VCR deployments
