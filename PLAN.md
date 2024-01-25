## Project Structure

```
vonage-cloud-runtime-cli/
├── LICENSE
├── Makefile
├── PLAN.md
├── README.md
├── docs    
│     ├── docs.go
│     ├── vcr.md
│     ├── vcr_app.md
│     ├── vcr_app_create.md
│     ├── vcr_app_generate-keys.md
│     ├── vcr_app_list.md
│     ├── vcr_configure.md
│     ├── vcr_debug.md
│     ├── vcr_deploy.md
│     ├── vcr_init.md
│     ├── vcr_instance.md
│     ├── vcr_instance_remove.md
│     ├── vcr_secret.md
│     ├── vcr_secret_create.md
│     ├── vcr_secret_remove.md
│     ├── vcr_secret_update.md
│     └── vcr_update.md
├── go.mod
├── go.sum
├── main.go
├── main_test.go
├── pkg
│     ├── api
│     │     ├── asset.go
│     │     ├── asset_test.go
│     │     ├── datastore.go
│     │     ├── datastore_test.go
│     │     ├── deployment.go
│     │     ├── deployment_test.go
│     │     ├── error.go
│     │     ├── graphql.go
│     │     ├── release.go
│     │     ├── release_test.go
│     │     ├── types.go
│     │     └── websocket.go
│     ├── cmdutil
│     │     ├── cmdutil.go
│     │     ├── cmdutil_test.go
│     │     ├── errors.go
│     │     └── factory.go
│     ├── config
│     │     ├── config.go
│     │     ├── config_test.go
│     │     ├── doc.go
│     │     ├── global_options.go
│     │     ├── manifest.go
│     │     ├── manifest_test.go
│     │     ├── secret.go
│     │     ├── secret_test.go
│     │     └── testdata
│     │         └── vcr.yaml
│     └── format
│         ├── format.go
│         └── format_test.go
├── tests
│     └── integration
│         ├── Dockerfile
│         ├── Dockerfile-clitool
│         ├── Dockerfile-graphql
│         ├── Dockerfile-mockserver
│         ├── docker-compose.yml
│         ├── init.sql
│         ├── mocks
│         │   └── main.go
│         ├── scripts
│         │   ├── run_clitool.sh
│         │   ├── run_hasura.sh
│         │   └── run_mockserver.sh
│         └── testdata
│           ├── config.yaml
│           └── vcr.yaml
├── testutil
│     ├── factory.go
│     ├── mocks
│     │     └── factory.go
│     └── testutil.go
├
└── vcr
    ├── app
    │     ├── app.go
    │     ├── create
    │     │     ├── create.go
    │     │     └── create_test.go
    │     ├── generatekeys
    │     │     ├── generatekeys.go
    │     │     └── generatekeys_test.go
    │     └── list
    │         ├── list.go
    │         └── list_test.go
    ├── configure
    │     ├── configure.go
    │     ├── configure_test.go
    │     └── testdata
    │         └── config.yaml
    ├── debug
    │     ├── client.go
    │     ├── client_test.go
    │     ├── command_gen_syscall_notwin.go
    │     ├── command_gen_syscall_win.go
    │     ├── commandgen.go
    │     ├── commandgen_test.go
    │     ├── debug.go
    │     ├── debug_test.go
    │     ├── prettyprint.go
    │     ├── server.go
    │     └── server_test.go
    ├── deploy
    │     ├── deploy.go
    │     ├── deploy_test.go
    │     └── testdata
    │         ├── test.tar.gz
    │         └── vcr.yaml
    ├── init
    │     ├── init.go
    │     ├── init_test.go
    │     └── testdata
    │         ├── test.zip
    │         └── vcr.yaml
    ├── instance
    │     ├── instance.go
    │     └── remove
    │         ├── remove.go
    │         └── remove_test.go
    ├── root
    │     ├── help.go
    │     ├── root.go
    │     └── root_test.go
    ├── secret
    │     ├── create
    │     │     ├── create.go
    │     │     └── create_test.go
    │     ├── remove
    │     │     ├── remove.go
    │     │     └── remove_test.go
    │     ├── secret.go
    │     └── update
    │         ├── update.go
    │         └── update_test.go
    └── upgrade
        ├── testdata
        │     └── vcr
        ├── upgrade.go
        └── upgrade_test.go
```
* The `vonage-cloud-runtime-cli` folder will contain all commands and sub commands of the CLI where the folder hierarchy will be the same as the command hierarchy. For example, `vcr app list` will be in `vcr/app/list/list.go`
* Each command should add the sub commands to its own command.