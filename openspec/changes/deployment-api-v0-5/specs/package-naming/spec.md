# Spec Delta

## Purpose

Defines the name `vcr deploy` gives the package it creates, and on which deployment API versions it sends one.

## ADDED Requirements

### Requirement: Releases call version v0.5
A released CLI SHALL send its deployment API requests under `/v0.5/`.

#### Scenario: A released build lists applications
- **WHEN** a release build runs `vcr app list`
- **THEN** it sends `GET <deployment API URL>/v0.5/applications`

### Requirement: The package is named after the project and the instance
On API version `v0.5` or later, `vcr deploy` SHALL send `name` in `POST /packages`, made from `<project name>-<instance name>`, using the names the deploy uses (flags over the manifest). On `v0.3` and `v0.4` the body SHALL NOT contain `name`.

#### Scenario: A deploy on v0.5
- **WHEN** a CLI built for `v0.5` deploys a manifest whose `project.name` is `shop` and whose `instance.name` is `dev`
- **THEN** its `POST /v0.5/packages` body has `"name": "shop-dev"`, plus the fields it sends on `v0.3`

#### Scenario: Flags override the manifest names
- **WHEN** a CLI built for `v0.5` deploys with `--project-name web --instance-name prod` and a manifest naming `shop` and `dev`
- **THEN** the body has `"name": "web-prod"`

#### Scenario: A deploy on v0.3
- **WHEN** a CLI built for `v0.3` deploys the same manifest
- **THEN** its `POST /v0.3/packages` body has no `name` field

### Requirement: The package name is sanitised
The CLI SHALL make the name contain only lowercase letters, digits and `-`, starting and ending with a letter or digit, as a Docker image name component requires: upper-case letters become lower case, each run of any other characters becomes one `-`, and leading and trailing `-` are removed. When nothing is left, the deploy SHALL end with exit code 1 and an error naming the project and instance names, and SHALL NOT create the package.

#### Scenario: Mixed case, digits and symbols
- **WHEN** a CLI built for `v0.5` deploys with project name `My_Shop 2` and instance name `Dev.EU`
- **THEN** the body has `"name": "my-shop-2-dev-eu"`

#### Scenario: Names of digits only
- **WHEN** a CLI built for `v0.5` deploys with project name `123` and instance name `456`
- **THEN** the body has `"name": "123-456"`

#### Scenario: Names with no letters or digits
- **WHEN** a CLI built for `v0.5` deploys with project name `__` and instance name `..`
- **THEN** the deploy exits 1 with an error naming `__` and `..`, and sends no `POST /packages`
