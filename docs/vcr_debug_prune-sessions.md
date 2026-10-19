## vcr debug prune-sessions

Remove all active debug sessions

### Synopsis

Remove all active debug sessions for the configured API key.

```
vcr debug prune-sessions [flags]
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

* [vcr debug](vcr_debug.md)	 - Run your application locally in debug mode with live VCR integration
