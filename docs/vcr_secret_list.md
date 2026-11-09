## vcr secret list

List all secrets

### Synopsis

List all secrets stored in your VCR account.

This command displays the names of all secrets. Secret values are never
shown for security reasons.

Use this to verify which secrets are available before referencing them
in your vcr.yml manifest.


```
vcr secret list [flags]
```

### Examples

```
# List all secrets
$ vcr secret list
✓ Found 3 secret(s):
  ℹ MY_API_KEY
  ℹ DATABASE_PASSWORD
  ℹ SSL_CERT

# No secrets configured
$ vcr secret list
! No secrets found

# Using the 'ls' alias
$ vcr secret ls

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

* [vcr secret](vcr_secret.md)	 - Manage secrets for VCR applications
