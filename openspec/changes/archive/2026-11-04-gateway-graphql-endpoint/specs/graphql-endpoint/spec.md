# Spec Delta

## Purpose

Defines which GraphQL endpoint the CLI uses for platform metadata, from flags, the config file and built-in defaults.

## ADDED Requirements

### Requirement: Endpoint precedence
The CLI SHALL use the `--graphql-endpoint` flag when set, otherwise the endpoint built from `--region` when set, otherwise the `graphql_endpoint` from the config file.

#### Scenario: Explicit endpoint wins
- **WHEN** a command runs with `--graphql-endpoint https://example.test/graphql --region aws.use1`
- **THEN** GraphQL requests go to `https://example.test/graphql`

#### Scenario: Config file endpoint is used as-is
- **WHEN** a command runs without `--graphql-endpoint` or `--region` and the config file sets `graphql_endpoint`
- **THEN** GraphQL requests go to that configured URL

### Requirement: Region endpoints go through the API gateway
For `--region`, the CLI SHALL build `https://api-<geo>.vonage.com/v1/vcr/<name>/api/graphql/v1/graphql`, where `<geo>` is `eu` for `euw1`, `us` for `use1`, and `ap` for `apse1` and `apse2`.

#### Scenario: EU region
- **WHEN** a command runs with `--region aws.euw1`
- **THEN** GraphQL requests go to `https://api-eu.vonage.com/v1/vcr/euw1/api/graphql/v1/graphql`

#### Scenario: US region
- **WHEN** a command runs with `--region aws.use1`
- **THEN** GraphQL requests go to `https://api-us.vonage.com/v1/vcr/use1/api/graphql/v1/graphql`

#### Scenario: AP region
- **WHEN** a command runs with `--region aws.apse2`
- **THEN** GraphQL requests go to `https://api-ap.vonage.com/v1/vcr/apse2/api/graphql/v1/graphql`

### Requirement: Unsupported regions are rejected
The CLI SHALL reject a `--region` value outside the supported set before making any request, with an error naming the supported regions.

#### Scenario: Unknown region
- **WHEN** a command runs with `--region aws.mars1`
- **THEN** the command fails with an error that lists `aws.euw1`, `aws.use1`, `aws.apse1` and `aws.apse2`, and no request is sent

#### Scenario: Standby region
- **WHEN** a command runs with `--region aws.euc1` or `--region aws.usw2`
- **THEN** the command fails with the same error and no request is sent

#### Scenario: Malformed region
- **WHEN** a command runs with `--region aws.euw1@example.com`
- **THEN** the command fails with the same error and no request is sent

### Requirement: Default endpoint for new configs
`vcr configure` SHALL write `https://api-eu.vonage.com/v1/vcr/euw1/api/graphql/v1/graphql` as `graphql_endpoint` when neither a flag nor the existing config file provides one.

#### Scenario: Fresh configure
- **WHEN** `vcr configure` runs with no config file and no endpoint flags
- **THEN** the written config file has `graphql_endpoint = https://api-eu.vonage.com/v1/vcr/euw1/api/graphql/v1/graphql`
