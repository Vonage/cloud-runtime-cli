#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.

$ErrorActionPreference = 'Stop'

# Releases from this version on carry checksums.txt and a signature.
$FirstVerified = 'v2.6.1'

# Release signing keys, one per line: <keyid> ssh-ed25519 <base64 public key>
# BEGIN RELEASE KEYS
$ReleaseKeys = @(
    'fd1652cb1b7cc0c3 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB6MAISQBLbNYWna92jPv0bjwWW4uNkeF7cQeJ1PN5FI'
)
# END RELEASE KEYS

$KeyPattern = '^([0-9a-f]{16}) ssh-ed25519 ([A-Za-z0-9+/=]+)\z'
$ReleasesUrl = 'https://github.com/Vonage/cloud-runtime-cli/releases'

$Tmp = $null

# Keep cmd from running an ssh-keygen found in the current directory.
$env:NoDefaultCurrentDirectoryInExePath = '1'

function Fail($Message) {
  if ($Tmp) {
    Remove-Item $Tmp -Recurse -Force -ErrorAction SilentlyContinue
  }
  [Console]::Error.WriteLine("Error: $Message")
  Exit 1
}

# Save-ReleaseFile NAME: download NAME of the resolved release into $Tmp.
# Returns $false when the server answers 404; any other failure ends the script.
function Save-ReleaseFile($Name) {
  $Target = Join-Path $Tmp $Name
  $ProgressPreference = 'SilentlyContinue'
  try {
    Invoke-WebRequest "$BaseUrl/$Name" -OutFile $Target -UseBasicParsing
    return $true
  } catch {
    Remove-Item $Target -Force -ErrorAction SilentlyContinue
    $Response = $_.Exception.Response
    if ($Response -and [int]$Response.StatusCode -eq 404) {
      return $false
    }
    Fail "downloading $Name failed: $($_.Exception.Message)"
  }
}

# ssh-keygen -Y exists from OpenSSH 8.0; older versions reject the option itself.
function Test-SshKeygenVerify {
  if (!(Get-Command ssh-keygen -ErrorAction SilentlyContinue)) {
    return $false
  }
  $Probe = (cmd /c "ssh-keygen -Y bogus < NUL 2>&1") | Out-String
  return !($Probe -match 'option -- .*Y')
}

function Confirm-Signature {
  $Found = $false
  foreach ($Key in $Keys) {
    $Parts = [regex]::Match($Key, $KeyPattern)
    $KeyId = $Parts.Groups[1].Value
    $Sig = "checksums.txt.$KeyId.sig"
    if (!(Save-ReleaseFile $Sig)) {
      continue
    }
    $Found = $true
    [IO.File]::WriteAllText((Join-Path $Tmp 'allowed_signers'), "vcr-release namespaces=`"vcr-release`" ssh-ed25519 $($Parts.Groups[2].Value)`n")
    # cmd feeds checksums.txt to ssh-keygen byte for byte; a PowerShell pipe would re-encode it.
    Push-Location $Tmp
    try {
      cmd /c "ssh-keygen -Y verify -f allowed_signers -I vcr-release -n vcr-release -s $Sig < checksums.txt > NUL 2>&1"
      $Code = $LASTEXITCODE
    } finally {
      Pop-Location
    }
    if ($Code -eq 0) {
      return
    }
  }
  if (!$Found) {
    Fail "no signature was published for vcr $Tag; refusing to install an unchecked download"
  }
  Fail "the signature of checksums.txt for vcr $Tag does not verify"
}

function Confirm-Archive {
  $Lines = @(Get-Content -LiteralPath (Join-Path $Tmp 'checksums.txt'))
  $Entries = @($Lines | Where-Object { $_ -cmatch "^[0-9a-f]{64}  $([regex]::Escape($Archive))\z" })
  if ($Entries.Count -ne 1) {
    Fail "checksums.txt has no usable entry for $Archive"
  }
  $Actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $ArchivePath).Hash.ToLowerInvariant()
  if ($Actual -cne $Entries[0].Substring(0, 64)) {
    Fail "checksum mismatch for $Archive; nothing was installed"
  }
}

$VCRInstall = $env:VCR_INSTALL
$BinDir = if ($VCRInstall) {
  "$VCRInstall\bin"
} else {
  "$Home\.vcr\bin"
}

$Archive = 'vcr_windows_amd64.tar.gz'
$VCRcliExe = "$BinDir\vcr_windows_amd64.exe"
$VCRExe = "$BinDir\vcr.exe"

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# Resolve the tag once so that every file comes from the same release.
$Requested = if ($v) {
  "$v"
} elseif ($args.Length -eq 1) {
  "$($args.Get(0))"
}
if ($Requested) {
  $Tag = if ($Requested.StartsWith('v')) { $Requested } else { "v$Requested" }
} else {
  try {
    $Latest = Invoke-WebRequest "$ReleasesUrl/latest" -Method Head -UseBasicParsing
  } catch {
    Fail "Error while fetching releases: $($_.Exception.Message)"
  }
  $LatestUrl = if ($Latest.BaseResponse.ResponseUri) {
    $Latest.BaseResponse.ResponseUri.AbsoluteUri
  } else {
    $Latest.BaseResponse.RequestMessage.RequestUri.AbsoluteUri
  }
  if (!$LatestUrl.StartsWith("$ReleasesUrl/tag/")) {
    Fail 'unexpected answer when looking up the latest vcr release'
  }
  $Tag = $LatestUrl.Substring("$ReleasesUrl/tag/".Length)
  if ($Tag.Contains('-')) {
    Fail 'the latest release is a prerelease; install one on purpose with -v'
  }
}
if ($Tag -cnotmatch '^v[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}(-[0-9A-Za-z.]+)?\z') {
  Fail 'invalid version'
}

$Prerelease = $Tag.Contains('-')
$Core = [version]($Tag.Substring(1).Split('-')[0])
$Verified = $Core -ge [version]($FirstVerified.Substring(1))
$BaseUrl = "$ReleasesUrl/download/$Tag"
$Keys = @($ReleaseKeys | Where-Object { $_ -cmatch $KeyPattern })

$Tmp = Join-Path ([IO.Path]::GetTempPath()) "vcr-install-$([Guid]::NewGuid().ToString('N'))"
$ArchivePath = Join-Path $Tmp $Archive

try {
  New-Item $Tmp -ItemType Directory | Out-Null
  if ($Verified) {
    if (!$Prerelease -and $Keys.Count -eq 0) {
      Fail "this installer carries no release signing key, so it cannot verify vcr $Tag. Download it from $ReleasesUrl and check it by hand"
    }
    if (!(Save-ReleaseFile 'checksums.txt')) {
      Fail "checksums.txt is missing for vcr $Tag; refusing to install an unchecked download"
    }
    $First = @(Get-Content -LiteralPath (Join-Path $Tmp 'checksums.txt') -TotalCount 1)
    if ($First.Count -ne 1 -or $First[0] -cne "# vcr $Tag") {
      Fail "checksums.txt does not belong to vcr $Tag"
    }
    if (!$Prerelease) {
      if (Test-SshKeygenVerify) {
        Confirm-Signature
      } else {
        [Console]::Error.WriteLine('! ssh-keygen with signature support (OpenSSH 8.0 or newer) was not found; the signature was not checked, relying on the checksum.')
      }
    }
  } else {
    [Console]::Error.WriteLine('! This release has no checksums; the download was not verified.')
  }

  Write-Output "Downloading vcr..."
  if (!(Save-ReleaseFile $Archive)) {
    Fail "Unable to find a vcr release for vcr_windows_amd64 - see github.com/Vonage/cloud-runtime-cli/releases for all versions"
  }
  if ($Verified) {
    Confirm-Archive
  }

  if (!(Test-Path $BinDir)) {
    New-Item $BinDir -ItemType Directory | Out-Null
  }

  tar -xzf $ArchivePath -C $BinDir

  If (Test-Path $VCRExe) {
      Remove-Item $VCRExe
  }

  Rename-Item $VCRcliExe $VCRExe
} finally {
  if (Test-Path $Tmp) {
    Remove-Item $Tmp -Recurse -Force -ErrorAction SilentlyContinue
  }
}

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Write-Output "vcr was installed successfully to" $VCRExe
Write-Output "Run 'vcr --help' to get started"
