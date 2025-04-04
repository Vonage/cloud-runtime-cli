## vcr app create

Create a Vonage application

```
vcr app create [flags]
```

### Examples

```
$ vcr app create --name App
✓ Application created
ℹ id: 1
ℹ name: App

```

### Options

```
  -m, --messages      Enable or disable messages
  -n, --name string   Name of the application
  -r, --rtc           Enable or disable RTC
  -v, --voice         Enable or disable voice
  -y, --yes           Skip prompts
```

### Options inherited from parent commands

```
      --api-key string            Vonage API key
      --api-secret string         Vonage API secret
      --config-file string        Path to config file (default is $HOME/.vcr-cli) (default "~/.vcr-cli")
      --graphql-endpoint string   Graphql endpoint used to fetch metadata
      --help                      Show help for command
      --region string             Vonage platform region
  -t, --timeout duration          Timeout for requests to Vonage platform (default 10m0s)
```

### SEE ALSO

* [vcr app](vcr_app.md)	 - Use app commands to manage Vonage applications

###### Auto generated by spf13/cobra on 26-Nov-2024
