## vcr instance

Manage deployed VCR instances

### Synopsis

Manage deployed VCR instances.

Instances are running deployments of your VCR applications. Each deployment
creates an instance that can be monitored and managed using these commands.

WHAT IS AN INSTANCE?
  An instance is a deployed version of your application running on VCR.
  Each instance has:
  • A unique instance ID
  • A service name (URL endpoint)
  • A project name and instance name (from your manifest)

AVAILABLE COMMANDS
  list (ls)     List all deployed instances
  log (logs)    View real-time logs from a running instance
  remove (rm)   Delete an instance and free its resources

IDENTIFYING INSTANCES
  Instances can be identified by either:
  • Instance ID: A unique UUID assigned during deployment
  • Project + Instance name: The combination from your vcr.yml manifest


### Examples

```
# List all instances
$ vcr instance list

# List instances filtered by service name
$ vcr instance list --filter "my-service"

# View logs for an instance by project and instance name
$ vcr instance log --project-name my-app --instance-name dev

# View logs for an instance by ID
$ vcr instance log --id 12345678-1234-1234-1234-123456789abc

# Remove an instance
$ vcr instance remove --project-name my-app --instance-name dev

# Remove an instance by ID with automatic confirmation
$ vcr instance rm --id 12345678-1234-1234-1234-123456789abc --yes

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
* [vcr instance list](vcr_instance_list.md)	 - List all deployed VCR instances
* [vcr instance log](vcr_instance_log.md)	 - Fetch logs from a deployed VCR instance
* [vcr instance remove](vcr_instance_remove.md)	 - Remove a deployed VCR instance
