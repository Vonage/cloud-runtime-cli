## vcr completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(vcr completion bash)

To load completions for every new session, execute once:

#### Linux:

	vcr completion bash > /etc/bash_completion.d/vcr

#### macOS:

	vcr completion bash > $(brew --prefix)/etc/bash_completion.d/vcr

You will need to start a new shell for this setup to take effect.


```
vcr completion bash
```

### Options

```
      --no-descriptions   disable completion descriptions
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

* [vcr completion](vcr_completion.md)	 - Generate the autocompletion script for the specified shell
