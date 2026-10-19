## vcr app list

List all Vonage applications in your account

### Synopsis

List all Vonage applications associated with your account.

This command displays a table of all Vonage applications, showing their IDs
and names. Use the application ID in your vcr.yml manifest file to link
your VCR deployment to a specific application.

Use the --filter flag to search for applications by name. The filter performs
a case-insensitive substring match.


```
vcr app list [flags]
```

### Examples

```
# List all applications
$ vcr app list
+--------------------------------------+----------------+
|                  ID                  |      NAME      |
+--------------------------------------+----------------+
| 12345678-1234-1234-1234-123456789abc | my-voice-app   |
| 87654321-4321-4321-4321-cba987654321 | my-sms-app     |
+--------------------------------------+----------------+

# List applications using the short alias
$ vcr app ls

# Filter applications by name
$ vcr app list --filter "voice"
+--------------------------------------+----------------+
|                  ID                  |      NAME      |
+--------------------------------------+----------------+
| 12345678-1234-1234-1234-123456789abc | my-voice-app   |
+--------------------------------------+----------------+

# Filter with partial match
$ vcr app list -f "prod"

```

### Options

```
  -f, --filter string   Filter applications by name (case-insensitive substring match)
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
