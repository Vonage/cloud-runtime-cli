# Spec Delta

## Purpose

Defines how the CLI chooses the platform API base URL from its flags and config file, and how it moves users off the deprecated GraphQL settings without breaking scripts or older CLIs.

## ADDED Requirements

### Requirement: Endpoint precedence
The CLI SHALL take the platform API base URL from the first of these that is set: the `--endpoint` flag; the URL mapped from the `--graphql-endpoint` flag; the production default when the `--region` flag is set; the `endpoint` key of the config file; the URL mapped from the `graphql_endpoint` key of the config file; the production default.

#### Scenario: The endpoint key wins over graphql_endpoint
- **WHEN** the config file sets both `endpoint = https://gw.example.test/v1/vcr/euw1/api/deployment` and a different `graphql_endpoint`, and a command runs with no endpoint flags
- **THEN** the CLI sends its requests to `https://gw.example.test/v1/vcr/euw1/api/deployment`

#### Scenario: The endpoint flag wins over the config file
- **WHEN** the config file sets `endpoint` and a command runs with `--endpoint https://other.example.test/api/deployment`
- **THEN** the CLI sends its requests to `https://other.example.test/api/deployment`

#### Scenario: The graphql-endpoint flag wins over the endpoint key
- **WHEN** the config file sets `endpoint` and a command runs with `--graphql-endpoint https://other.example.test/api/graphql/v1/graphql`
- **THEN** the CLI sends its requests to `https://other.example.test/api/deployment`

### Requirement: The endpoint flag is documented
The `--endpoint` flag SHALL appear in the help of every command, and the help text of `--graphql-endpoint` SHALL say that it is deprecated in favour of `--endpoint`.

#### Scenario: Help lists both flags
- **WHEN** a user runs `vcr app list --help`
- **THEN** the global flags list `--endpoint`, and the `--graphql-endpoint` line says it is deprecated and names `--endpoint`

### Requirement: Notice for the graphql-endpoint flag
When the `--graphql-endpoint` flag decides the platform URL of a command that reads the config file, the CLI SHALL print one line on stderr saying the flag is deprecated and naming its replacement, and SHALL NOT ask a question. Commands that do not read the config file are `vcr configure`, `vcr upgrade`, help, completion and `--version`.

#### Scenario: Gateway URL on the command line
- **WHEN** a command runs with `--graphql-endpoint https://gw.example.test/v1/vcr/euw1/api/graphql/v1/graphql`
- **THEN** stderr has one line saying `--graphql-endpoint` is deprecated and to use `--endpoint https://gw.example.test/v1/vcr/euw1/api/deployment` instead

#### Scenario: Production URL on the command line
- **WHEN** a command runs with `--graphql-endpoint https://graphql.use1.runtime.vonage.cloud/v1/graphql`
- **THEN** stderr has one line saying `--graphql-endpoint` is deprecated and not needed, and to choose the region with `--region`

#### Scenario: No question on an interactive terminal
- **WHEN** a command runs with `--graphql-endpoint` on an interactive terminal in rich output
- **THEN** the CLI prints the notice and asks nothing

#### Scenario: The flag wins, so the file is not mentioned
- **WHEN** a command runs with `--graphql-endpoint` and the config file sets only `graphql_endpoint`
- **THEN** the CLI prints the notice for the flag only, and neither asks about nor mentions the config file

### Requirement: Migration output leaves the command's result alone
Notices and questions about the endpoint SHALL NOT change a command's exit code, and notices SHALL go to stderr, so the command's stdout is the same as without them.

#### Scenario: Same result as the new flag
- **WHEN** a command runs with `--graphql-endpoint https://gw.example.test/v1/vcr/euw1/api/graphql/v1/graphql`
- **THEN** its stdout and exit code are those of the same command run with `--endpoint https://gw.example.test/v1/vcr/euw1/api/deployment`

#### Scenario: Same result after a notice about the file
- **WHEN** a command prints the notice about `graphql_endpoint` in the config file
- **THEN** its stdout and exit code are those of the same command run with a file that also sets `endpoint`

### Requirement: Asking to migrate graphql_endpoint
When the `graphql_endpoint` key of a config file without an `endpoint` key decides the platform URL, and the output is rich or static with stdin a terminal, a command that reads the config file SHALL ask on stdout, before it contacts the platform, whether to add `endpoint = <mapped URL>` to the named file, as a `[y/n]` question.

#### Scenario: Yes on an interactive terminal
- **WHEN** the config file has `graphql_endpoint = https://gw.example.test/v1/vcr/euw1/api/graphql/v1/graphql` and no `endpoint`, and a user runs `vcr app list` in rich output and answers `y`
- **THEN** the file gains `endpoint = https://gw.example.test/v1/vcr/euw1/api/deployment` and keeps `graphql_endpoint`, `default_region`, the credentials and every other key, section and comment
- **AND** one line says what was written, and the app list is shown

#### Scenario: No on an interactive terminal
- **WHEN** the same user answers `n`, or stdin ends
- **THEN** the file is unchanged, the app list is shown, and the next interactive run asks again

#### Scenario: The file cannot be written
- **WHEN** a user answers `y` and the config file is read-only
- **THEN** stderr has one warning line saying the file could not be updated, and the command runs

#### Scenario: Production graphql_endpoint
- **WHEN** the config file has `graphql_endpoint = https://graphql.euw1.runtime.vonage.cloud/v1/graphql` and no `endpoint`
- **THEN** the URL offered is the production default deployment API URL

### Requirement: Telling scripts to migrate graphql_endpoint
When the `graphql_endpoint` key of a config file without an `endpoint` key decides the platform URL, and the output is classic or stdin is not a terminal, a command that reads the config file SHALL print one line on stderr naming the file and the `endpoint = <mapped URL>` line to add, and SHALL NOT ask.

#### Scenario: Pipeline or classic output
- **WHEN** the gateway-form config file is used by `vcr app list` with stdout piped, or with `VCR_UI=classic`
- **THEN** stderr has one line naming the config file and saying to add `endpoint = https://gw.example.test/v1/vcr/euw1/api/deployment`
- **AND** nothing waits for input

### Requirement: No migration output when nothing needs migrating
The CLI SHALL neither ask nor print anything about the endpoint when the config file sets `endpoint`, when `--endpoint` decides the URL, or for a command that does not read the config file.

#### Scenario: Already migrated
- **WHEN** the config file sets `endpoint`
- **THEN** the CLI neither asks nor prints anything about the endpoint

#### Scenario: Commands that do not read the config file
- **WHEN** a user runs `vcr configure`, `vcr upgrade`, `vcr --version`, help or completion with a config file that sets only `graphql_endpoint`
- **THEN** the CLI neither asks nor prints anything about the endpoint

### Requirement: Configure writes the endpoint key
`vcr configure` SHALL write an `endpoint` key holding the platform API base URL it used to list the regions, and SHALL keep writing `graphql_endpoint` as before.

#### Scenario: Fresh configure
- **WHEN** a user runs `vcr configure` with no config file and no endpoint flags
- **THEN** the new file has `endpoint` set to the production default deployment API URL, and `graphql_endpoint` set as before

#### Scenario: Configure with a gateway graphql endpoint
- **WHEN** a user runs `vcr configure --graphql-endpoint https://gw.example.test/v1/vcr/euw1/api/graphql/v1/graphql`
- **THEN** the file has `endpoint = https://gw.example.test/v1/vcr/euw1/api/deployment` and `graphql_endpoint = https://gw.example.test/v1/vcr/euw1/api/graphql/v1/graphql`
