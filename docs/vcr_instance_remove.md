## vcr instance remove

Remove a deployed VCR instance

### Synopsis

Remove a deployed VCR instance.

This command permanently deletes an instance from the VCR platform, stopping
the running application and freeing all associated resources.

IDENTIFYING THE INSTANCE
  You can identify the instance to remove using either:
  • --id: The unique instance UUID (from deployment output)
  • --project-name + --instance-name: The combination from your manifest

WARNING: This action is irreversible. All data associated with the instance
will be permanently deleted. You will be prompted for confirmation unless
--yes is specified.


```
vcr instance remove [flags]
```

### Examples

```
# Remove by project and instance name
$ vcr instance remove --project-name my-app --instance-name dev
? Are you sure you want to remove instance with id="abc123" and service_name="my-service"? Yes
✓ Instance "abc123" successfully removed

# Remove using the short alias
$ vcr instance rm -p my-app -n dev

# Remove by instance ID
$ vcr instance remove --id 12345678-1234-1234-1234-123456789abc

# Skip confirmation prompt (useful for CI/CD)
$ vcr instance rm --project-name my-app --instance-name dev --yes

```

### Options

```
  -i, --id string              Instance UUID (alternative to project-name + instance-name)
  -n, --instance-name string   Instance name (requires --project-name)
  -p, --project-name string    Project name (requires --instance-name)
  -y, --yes                    Skip confirmation prompt (use with caution)
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

* [vcr instance](vcr_instance.md)	 - Manage deployed VCR instances
