## vcr secret

Manage secrets for VCR applications

### Synopsis

Manage secrets for VCR applications.

Secrets allow you to securely store sensitive values like API keys, passwords,
and tokens that your VCR applications need at runtime. Secrets are:
  • Encrypted at rest
  • Injected as environment variables into your deployed instances
  • Scoped to your Vonage account

USING SECRETS IN YOUR APPLICATION
  Reference secrets in your vcr.yml manifest:

  instance:
    environment:
      - name: MY_API_KEY
        secret: MY_SECRET_NAME   # References a secret

  The secret value is then available in your application as the
  environment variable MY_API_KEY.

AVAILABLE COMMANDS
  create (add)   Create a new secret
  list (ls)      List all secrets
  update         Update an existing secret's value
  remove (rm)    Delete a secret

SECRET NAMING
  Secret names must be valid environment variable names:
  • Alphanumeric characters and underscores only
  • Cannot start with a number
  • Case-sensitive


### Examples

```
# Create a secret with a value
$ vcr secret create --name MY_API_KEY --value "sk-12345..."

# Create a secret from a file (useful for certificates, multi-line values)
$ vcr secret create --name SSL_CERT --filename ./cert.pem

# Update a secret's value
$ vcr secret update --name MY_API_KEY --value "sk-new-key..."

# Remove a secret
$ vcr secret remove --name MY_API_KEY

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
* [vcr secret create](vcr_secret_create.md)	 - Create a new secret
* [vcr secret list](vcr_secret_list.md)	 - List all secrets
* [vcr secret remove](vcr_secret_remove.md)	 - Remove a secret
* [vcr secret update](vcr_secret_update.md)	 - Update an existing secret's value
