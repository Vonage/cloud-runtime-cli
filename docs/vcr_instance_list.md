## vcr instance list

List all deployed VCR instances

### Synopsis

List all deployed VCR instances.

This command displays a table of all non-deleted VCR instances, showing their
IDs, linked API application IDs, instance names and service names.


```
vcr instance list [flags]
```

### Examples

```
# List all instances
$ vcr instance list
+--------------------------------------+--------------------------------------+---------------+--------------+
|             INSTANCE ID              |         API APPLICATION ID           | INSTANCE NAME | SERVICE NAME |
+--------------------------------------+--------------------------------------+---------------+--------------+
| 12345678-1234-1234-1234-123456789abc | 87654321-4321-4321-4321-cba987654321 | dev           | my-service   |
+--------------------------------------+--------------------------------------+---------------+--------------+

# List instances using the short alias
$ vcr instance ls

# Filter instances by service name
$ vcr instance list --filter "my-service"

# Filter with short flag
$ vcr instance list -f "prod"

```

### Options

```
  -f, --filter string   Filter instances by service name (case-insensitive substring match)
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

* [vcr instance](vcr_instance.md)	 - Manage deployed VCR instances
