## vcr app capabilities add

Add default capabilities to the application-capabilities block

### Synopsis

Add default capabilities to the application-capabilities block of vcr.yml.

This command writes the default of each capability you name into the manifest's
application-capabilities block. Webhook addresses use {{instance_url}}, the URL
of your deployed instance. The rest of the manifest is kept as written. When the
manifest has instance.capabilities, it is removed: the block replaces it.

CAPABILITIES
  • Voice         (--voice)         - /voice/answer and /voice/event
  • Messages      (--messages)      - version v1, /messages/inbound and /messages/status
  • RTC           (--rtc)           - /rtc/event
  • Verify        (--verify)        - version v2, /verify/status
  • Video         (--video)         - /video/session-created and five more, all active
  • VBC           (--vbc)           - an empty mapping
  • Meetings      (--meetings)      - /meetings/recording-changed, /meetings/room-changed
                                      and /meetings/session-changed
  • Network APIs  (--network-apis)  - the values of --network-application-id and --redirect-uri

Every webhook except Video's is sent with POST. A capability already in the block
is kept and reported as skipped, unless --force is given.

NOTE: The manifest is the one vcr deploy would use in the current directory, or
the one named by --filename. With --print, the resulting block is printed instead
and no file is written; no manifest is needed.


```
vcr app capabilities add [flags]
```

### Examples

```
# Add the default Voice and Messages capabilities
$ vcr app capabilities add --voice --messages
✓ Added voice to application-capabilities in /path/to/project/vcr.yml
✓ Added messages to application-capabilities in /path/to/project/vcr.yml

# Print the block without writing the manifest
$ vcr app capabilities add --rtc --print

# Replace a capability that is already configured with its default
$ vcr app capabilities add --voice --force

# Add Network APIs
$ vcr app capabilities add --network-apis --network-application-id my-network-app --redirect-uri https://example.com/callback

```

### Options

```
  -f, --filename string                 Path to manifest file (default: vcr.yml in project directory)
      --force                           Replace capabilities that are already configured with their defaults
      --meetings                        Add the default Meetings capability
      --messages                        Add the default Messages capability
      --network-apis                    Add the Network APIs capability (needs --network-application-id and --redirect-uri)
      --network-application-id string   Network application ID for --network-apis
      --print                           Print the resulting application-capabilities block instead of writing the manifest
      --redirect-uri string             Redirect URI for --network-apis
      --rtc                             Add the default RTC (Real-Time Communication) capability
      --vbc                             Add the default VBC (Vonage Business Communications) capability
      --verify                          Add the default Verify capability
      --video                           Add the default Video capability
      --voice                           Add the default Voice capability
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

* [vcr app capabilities](vcr_app_capabilities.md)	 - Manage the application-capabilities block in vcr.yml
