#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.

$ErrorActionPreference = 'Stop'

$Version = if ($v) {
  $v
} elseif ($args.Length -eq 1) {
  "download/" + $args.Get(0)
} else {
  "latest/download"
}

$VCRInstall = $env:VCR_INSTALL
$BinDir = if ($VCRInstall) {
  "$VCRInstall\bin"
} else {
  "$Home\.vcr\bin"
}

$VCRTar = "$BinDir\vcr_windows_amd64.tar.gz"
$VCRcliExe = "$BinDir\vcr_windows_amd64.exe"
$VCRExe = "$BinDir\vcr.exe"
$VCRUri = "https://github.com/Vonage/cloud-runtime-cli/releases/$Version/vcr_windows_amd64.tar.gz"

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

try {
  $Response = Invoke-WebRequest $VCRUri -UseBasicParsing
}
catch {
  $StatusCode = $_.Exception.Response.StatusCode.value__
  if ($StatusCode -eq 404) {
    Write-Error "Unable to find a vcr release for vcr_windows_amd64 - see github.com/Vonage/cloud-runtime-cli/releases for all versions"
  } else {
    $Request = $_.Exception
    Write-Error "Error while fetching releases: $Request"
  }
  Exit 1
}

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

$prevProgressPreference = $ProgressPreference
try {
  if ($PSVersionTable.PSVersion.Major -lt 7) {
    Write-Output "Downloading vcr..."
    $ProgressPreference = "SilentlyContinue"
  }

  Invoke-WebRequest $VCRUri -OutFile $VCRTar -UseBasicParsing
} finally {
  $ProgressPreference = $prevProgressPreference
}

tar -xzf $VCRTar -C $BinDir

If (Test-Path $VCRExe) {
    Remove-Item $VCRExe
}

Rename-Item $VCRcliExe $VCRExe

Remove-Item $VCRTar

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Write-Output "vcr was installed successfully to" $VCRExe
Write-Output "Run 'vcr --help' to get started"