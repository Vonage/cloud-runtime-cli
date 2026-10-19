## vcr configure

Configure VCR CLI with your Vonage API credentials

### Synopsis

Configure the VCR CLI with your Vonage API credentials.

This interactive command sets up the VCR CLI by prompting you for:
  • Vonage API Key     - Found in your Vonage API Dashboard
  • Vonage API Secret  - Found in your Vonage API Dashboard
  • Default Region     - The Vonage Cloud Runtime region for deployments

On successful configuration, a configuration file is created at $HOME/.vcr-cli
(or the path specified by --config-file).

PREREQUISITES
  You need a Vonage API account to use VCR. Get your API credentials from:
  https://dashboard.nexmo.com/settings

CONFIGURATION FILE
  The configuration file stores your credentials and preferences. You can have
  multiple configuration files for different accounts/environments by using
  the --config-file flag with other commands.

NOTE: The VCR CLI requires configuration before any other commands will work.
Run this command first after installing the CLI.


```
vcr configure [flags]
```

### Examples

```
# Configure the CLI interactively
$ vcr configure
? Enter your Vonage api key: abc123
? Enter your Vonage api secret: ********
? Select your Vonage region: aws.euw1 - AWS Europe (Ireland)
✓ New configuration file written to /Users/you/.vcr-cli

# Use a custom configuration file path
$ vcr configure --config-file ~/.vcr-cli-staging

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

* [vcr](vcr.md)	 - Streamline your Vonage Cloud Runtime development and management tasks with VCR
