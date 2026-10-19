## vcr app remove

Remove a Vonage application

### Synopsis

Remove a Vonage application from your account.

This command permanently removes a Vonage application and its associated
credentials. Any VCR instances linked to this application will lose their
authentication credentials on next restart.

WARNING: This action is irreversible. Make sure no running instances
depend on this application before removing it.


```
vcr app remove <applicationID> [flags]
```

### Examples

```
# Remove an application (will prompt for confirmation)
$ vcr app remove 12345678-1234-1234-1234-123456789abc

# Remove without confirmation prompt (useful for CI/CD)
$ vcr app remove 12345678-1234-1234-1234-123456789abc --yes

# Using the short alias
$ vcr app rm 12345678-1234-1234-1234-123456789abc --yes

```

### Options

```
  -y, --yes   Skip confirmation prompt (use with caution)
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
