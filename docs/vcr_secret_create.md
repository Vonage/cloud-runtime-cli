## vcr secret create

Create a new secret

### Synopsis

Create a new secret for use in VCR applications.

Secrets are securely stored and can be referenced in your vcr.yml manifest
to inject sensitive values as environment variables in your deployed instances.

SECRET VALUE INPUT
  You can provide the secret value in two ways:
  • --value: Pass the value directly (be careful with shell history)
  • --filename: Read the value from a file (recommended for multi-line values)

  If neither is provided, you will be prompted to enter the value interactively.

SECRET NAMING RULES
  • Must be a valid environment variable name
  • Alphanumeric characters and underscores only
  • Cannot start with a number
  • Case-sensitive (MY_SECRET and my_secret are different)

NOTE: Secret names must be unique within your account. If a secret with the
same name already exists, this command will fail. Use 'vcr secret update' instead.


```
vcr secret create [flags]
```

### Examples

```
# Create a secret with a direct value
$ vcr secret create --name MY_API_KEY --value "sk-12345abcde"
✓ Secret "MY_API_KEY" created

# Create a secret from a file
$ vcr secret create --name SSL_CERT --filename ./server.crt

# Create a secret with interactive value input (value not shown in history)
$ vcr secret create --name DATABASE_PASSWORD
? Enter value for secret "DATABASE_PASSWORD": ********
✓ Secret "DATABASE_PASSWORD" created

# Using short flags
$ vcr secret create -n WEBHOOK_SECRET -v "whsec_xyz123"

# Using the 'add' alias
$ vcr secret add --name MY_TOKEN --value "token123"

```

### Options

```
  -f, --filename string   Path to file containing the secret value
  -n, --name string       Secret name (must be valid env var name, required)
  -v, --value string      Secret value (or use --filename for file input)
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
