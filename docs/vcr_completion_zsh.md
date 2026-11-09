## vcr completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(vcr completion zsh)

To load completions for every new session, execute once:

#### Linux:

	vcr completion zsh > "${fpath[1]}/_vcr"

#### macOS:

	vcr completion zsh > $(brew --prefix)/share/zsh/site-functions/_vcr

You will need to start a new shell for this setup to take effect.


```
vcr completion zsh [flags]
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
      --endpoint string           Platform API base URL (overrides --graphql-endpoint and the config file)
      --graphql-endpoint string   Deprecated: use --endpoint. GraphQL endpoint used to fetch metadata
      --help                      Show help for command
      --region string             Vonage platform region
  -t, --timeout duration          Overall deadline for the command, including waiting for a build to finish (e.g. 20m) (default 10m0s)
```

### SEE ALSO

* [vcr completion](vcr_completion.md)	 - Generate the autocompletion script for the specified shell
