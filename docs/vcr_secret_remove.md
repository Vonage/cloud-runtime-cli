## vcr secret remove

Remove a secret

### Synopsis

Remove a secret from your VCR account.

This command permanently deletes a secret. Any deployed instances that
reference this secret will fail to access the value on their next restart.

WARNING: This action is irreversible. Make sure no running instances
depend on this secret before removing it.

BEFORE REMOVING
  1. Check if any vcr.yml manifests reference this secret
  2. Update or redeploy affected instances first
  3. Then remove the secret


```
vcr secret remove [flags]
```

### Examples

```
# Remove a secret by name
$ vcr secret remove --name MY_API_KEY
✓ Secret "MY_API_KEY" successfully removed

# Using the short flag
$ vcr secret remove -n DATABASE_PASSWORD

# Using the 'rm' alias
$ vcr secret rm --name OLD_TOKEN

```

### Options

```
  -n, --name string   Name of the secret to remove (required)
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

* [vcr secret](vcr_secret.md)	 - Manage secrets for VCR applications
