# Vonage cloud runtime - CLI

![Actions](https://github.com/Vonage/vonage-cloud-runtime-cli/workflows/Release%20CLI/badge.svg)

<img src="https://developer.nexmo.com/assets/images/Vonage_Nexmo.svg" height="48px" alt="Nexmo is now known as Vonage" />

Vonage cloud runtime - CLI (VCR) is a powerful command-line interface designed to streamline
and simplify the development and management of applications on
the [Vonage Cloud Runtime platform](https://developer.vonage.com/en/cloud-runtime). It is still under active development. Issues, pull requests and other input is very welcome.

* [Installation](#installation)
* [Project Structure](#project-structure)
* [Usage](#usage)
* [Contributions](#contributions)
* [Getting Help](#getting-help)

## Installation

Find current and past releases on the [releases page](https://github.com/Vonage/vonage-cloud-runtime-cli/releases).

### macOS

#### M1 (ARM)
```
curl -L -O https://github.com/Vonage/vonage-cloud-runtime-cli/releases/latest/download/vcr_darwin_arm64.tar.gz
tar -xvf vcr_darwin_arm64.tar.gz
sudo mv vcr_darwin_arm64 /usr/local/bin/vcr
```
#### Intel
```
curl -L -O https://github.com/Vonage/vonage-cloud-runtime-cli/releases/latest/download/vcr_darwin_amd64.tar.gz
tar -xvf vcr_darwin_amd64.tar.gz
sudo mv vcr_darwin_amd64 /usr/local/bin/vcr
```

### Linux
```
curl -L -O https://github.com/Vonage/vonage-cloud-runtime-cli/releases/latest/download/vcr_linux_amd64.tar.gz
tar -xvf vcr_linux_amd64.tar.gz
sudo mv vcr_linux_amd64 /usr/local/bin/vcr
```

### Windows
```
mkdir .vcr
cd .vcr
curl -L -O https://github.com/Vonage/vonage-cloud-runtime-cli/releases/latest/download/vcr_windows_amd64.tar.gz
tar -xzf vcr_windows_amd64.tar.gz
ren vcr_windows_amd64.exe vcr.exe
setx PATH "%PATH%;%cd%"
Close and reopen command prompt to reload your PATH.
```


## Project Structure

[Structure](PLAN.md) of the project

## Usage

Usage examples are in the `docs/` folder - [vcr](docs/vcr.md)

## Contributions

Yes please! This command-line interface is open source, community-driven, and benefits greatly from the input of its users.

Please make all your changes on a branch, and open a pull request, these are welcome and will be reviewed with delight! If it's a big change, it is recommended to open an issue for discussion before you start.

All changes require tests to go with them.

## Getting Help

We love to hear from you so if you have questions, comments or find a bug in the project, let us know! You can either:

* Open an [issue on this repository](https://github.com/Vonage/vonage-cloud-runtime-cli/issues)
* Tweet at us! We're [@VonageDev on Twitter](https://twitter.com/VonageDev)
* Or [join the Vonage Community Slack](https://developer.nexmo.com/community/slack)

## License

This library is released under the [Apache 2.0 License][license]

[license]: LICENSE