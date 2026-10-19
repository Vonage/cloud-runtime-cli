#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
set -e

# Releases from this version on carry checksums.txt and a signature.
FIRST_VERIFIED=v2.6.1

# Release signing keys, one per line: <keyid> ssh-ed25519 <base64 public key>
# BEGIN RELEASE KEYS
RELEASE_KEYS='
fd1652cb1b7cc0c3 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB6MAISQBLbNYWna92jPv0bjwWW4uNkeF7cQeJ1PN5FI
'
# END RELEASE KEYS

releases_url="https://github.com/Vonage/cloud-runtime-cli/releases"

version=""
path=""
while test $# -gt 0; do
  case "$1" in
    -h|--help)
      echo "options:"
      echo "-h, --help                show brief help"
      echo "-v, --version=VERSION       specify a version to download"
      echo "-o, --output-dir=DIR      specify a directory to install"
      exit 0
      ;;
    -v)
      shift
      if test $# -gt 0; then
        version=$1
      else
        echo "no version specified"
        exit 1
      fi
      shift
      ;;
    --version*)
      version=${1#*=}
      echo "version:$version"
      shift
      ;;
    -o)
      shift
      if test $# -gt 0; then
        path=$1
      else
        echo "no output dir specified"
        exit 1
      fi
      shift
      ;;
    --output-dir*)
      path=${1#*=}
      echo "path:$path"
      shift
      ;;
    *)
      echo "Error: Invalid argument $1" 1>&2
      exit 1
      ;;
  esac
done

die() {
  echo "Error: $*" 1>&2
  exit 1
}

# fetch FILE: download FILE of the resolved release into $tmp.
# Returns 1 when the server answers 404; any other failure ends the script.
fetch() {
  fetch_rc=0
  fetch_code=$(curl -q --fail --location --proto =https --tlsv1.2 --silent --show-error \
    --write-out '%{http_code}' -o "$tmp/$1" "$base_url/$1" 2>"$tmp/curl.err") || fetch_rc=$?
  [ "$fetch_rc" -eq 0 ] && return 0
  rm -f "$tmp/$1"
  [ "$fetch_rc" -eq 22 ] && [ "$fetch_code" = 404 ] && return 1
  cat "$tmp/curl.err" 1>&2
  die "downloading $1 failed (curl exit $fetch_rc, HTTP $fetch_code)"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{ print $NF }'
  else
    die "no SHA-256 tool found (need sha256sum, shasum or openssl)"
  fi
}

# version_ge A B: true when the X.Y.Z version A is at least B. Plain sh, no sort -V.
version_ge() {
  ver_ifs=$IFS
  IFS=.
  # shellcheck disable=SC2086 # splitting on dots is the point
  set -- $1 $2
  IFS=$ver_ifs
  [ "$1" -gt "$4" ] && return 0
  [ "$1" -lt "$4" ] && return 1
  [ "$2" -gt "$5" ] && return 0
  [ "$2" -lt "$5" ] && return 1
  [ "$3" -ge "$6" ]
}

# ssh-keygen -Y exists from OpenSSH 8.0; older versions reject the option itself.
have_ssh_verify() {
  command -v ssh-keygen >/dev/null 2>&1 || return 1
  probe=$(ssh-keygen -Y bogus 2>&1 </dev/null) || true
  case $probe in
    *"option -- "*Y*) return 1 ;;
  esac
  return 0
}

verify_signature() {
  allowed="$tmp/allowed_signers"
  found=0
  while read -r keyid _ keyb64; do
    fetch "checksums.txt.$keyid.sig" || continue
    found=1
    sig="$tmp/checksums.txt.$keyid.sig"
    printf 'vcr-release namespaces="vcr-release" ssh-ed25519 %s\n' "$keyb64" >"$allowed"
    if ssh-keygen -Y verify -f "$allowed" -I vcr-release -n vcr-release -s "$sig" <"$tmp/checksums.txt" >/dev/null 2>&1; then
      return 0
    fi
  done <"$tmp/keys"
  if [ "$found" -eq 0 ]; then
    die "no signature was published for vcr $tag; refusing to install an unchecked download"
  fi
  die "the signature of checksums.txt for vcr $tag does not verify"
}

check_archive() {
  expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
  case $expected in
    ""|*[!0-9a-f]*) die "checksums.txt has no usable entry for $archive" ;;
  esac
  [ "${#expected}" -eq 64 ] || die "checksums.txt has a malformed entry for $archive"
  actual=$(sha256_of "$tmp/$archive")
  [ "$actual" = "$expected" ] || die "checksum mismatch for $archive; nothing was installed"
}

main() {

  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)
  if [ "$arch" = "x86_64" ]; then
    arch="amd64"
  elif [ "$arch" = "aarch64" ]; then
    arch="arm64"
  fi
  vcr_binary="vcr_${os}_${arch}"
  archive="${vcr_binary}.tar.gz"

  # Resolve the tag once so that every file comes from the same release.
  if [ -n "$version" ]; then
    tag=$version
    case $tag in v*) ;; *) tag="v$tag" ;; esac
  else
    latest_url=$(curl -q --fail --proto =https --tlsv1.2 --silent --show-error \
      --write-out '%{redirect_url}' -o /dev/null "$releases_url/latest") ||
      die "unable to look up the latest vcr release - see github.com/Vonage/cloud-runtime-cli/releases"
    case $latest_url in
      "$releases_url/tag/"*) tag=${latest_url#"$releases_url/tag/"} ;;
      *) die "unexpected answer when looking up the latest vcr release" ;;
    esac
    case $tag in
      *-*) die "the latest release is a prerelease; install one on purpose with -v" ;;
    esac
  fi
  case $tag in
    *[!0-9A-Za-z.-]*) die "invalid version" ;;
  esac
  printf '%s\n' "$tag" | grep -Eq '^v[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}(-[0-9A-Za-z.]+)?$' || die "invalid version"

  core=${tag#v}
  core=${core%%-*}
  case $tag in
    *-*) prerelease=1 ;;
    *) prerelease=0 ;;
  esac
  base_url="$releases_url/download/$tag"

  tmp=$(mktemp -d "${TMPDIR:-/tmp}/vcr-install.XXXXXX") || die "unable to create a temporary directory"
  trap 'rm -rf "$tmp"' EXIT
  trap 'exit 1' HUP INT TERM

  verified=0
  if version_ge "$core" "${FIRST_VERIFIED#v}"; then
    verified=1
    printf '%s\n' "$RELEASE_KEYS" | grep -E '^[0-9a-f]{16} ssh-ed25519 [A-Za-z0-9+/=]+$' >"$tmp/keys" || true
    if [ "$prerelease" -eq 0 ] && [ ! -s "$tmp/keys" ]; then
      die "this installer carries no release signing key, so it cannot verify vcr $tag. Download it from $releases_url and check it by hand"
    fi
    fetch checksums.txt || die "checksums.txt is missing for vcr $tag; refusing to install an unchecked download"
    [ "$(head -n 1 "$tmp/checksums.txt")" = "# vcr $tag" ] || die "checksums.txt does not belong to vcr $tag"
    if [ "$prerelease" -eq 0 ]; then
      if have_ssh_verify; then
        verify_signature
      else
        echo "! ssh-keygen with signature support (OpenSSH 8.0 or newer) was not found; the signature was not checked, relying on the checksum." 1>&2
      fi
    fi
  else
    echo "! This release has no checksums; the download was not verified." 1>&2
  fi

  fetch "$archive" || die "Unable to find a vcr release for vcr_${os}_${arch} - see github.com/Vonage/cloud-runtime-cli/releases for all versions"
  if [ "$verified" -eq 1 ]; then
    check_archive
  fi

  vcr_install="${VCR_INSTALL:-$HOME/.vcr}"

  bin_dir="$vcr_install/bin"
  exe="$bin_dir/vcr"
  sys_exe="/usr/local/bin/vcr"

  mkdir -p "$bin_dir"

  if [ -n "$path" ]; then
    echo "Path provided: $path"
    mkdir -p "$path"
  fi

  tar -C "$tmp" -xzf "$tmp/$archive"
  chmod +x "$tmp/$vcr_binary"

  cp "$tmp/$vcr_binary" "$exe"

  # If custom path is provided, use it
  if [ -n "$path" ]; then
    if mv "$tmp/$vcr_binary" "$path/vcr"; then
      echo "vcr was installed successfully to $path/vcr"
      echo "Run '$path/vcr --help' to get started"
      case $SHELL in
      /bin/zsh) shell_profile=".zshrc" ;;
      *) shell_profile=".bash_profile" ;;
      esac
      echo "Or manually add the directory to your \$HOME/$shell_profile (or similar)"
      echo "  export VCR_INSTALL=\"$path\""
      echo "  export PATH=\"\$VCR_INSTALL:\$PATH\""
      exit 0
    else
      exit 1
    fi
  fi

  # Try to install to system-wide location first
  if mv "$tmp/$vcr_binary" "$sys_exe" 2>/dev/null; then
    echo "vcr was installed successfully to $sys_exe"
    echo "Run 'vcr --help' to get started"
    exit 0
  fi

  # If system-wide fails, try user-local directories
  for user_path in "$HOME/.local/bin" "$HOME/bin" "$HOME/.vcr/bin"; do
    if mkdir -p "$user_path" 2>/dev/null && mv "$tmp/$vcr_binary" "$user_path/vcr" 2>/dev/null; then
      echo "vcr was installed successfully to $user_path"

      # Check if the path is already in PATH
      case ":$PATH:" in
        *":$user_path:"*)
          echo "Run 'vcr --help' to get started"
          ;;
        *)
          case $SHELL in
            /bin/zsh) shell_profile=".zshrc" ;;
            *)        shell_profile=".bash_profile" ;;
          esac
          echo ""
          echo "⚠️  Warning: $user_path is not in your \$PATH"
          echo "Add the following to your \$HOME/$shell_profile:"
          echo "  export PATH=\"$user_path:\$PATH\""
          echo ""
          echo "Then you can run: vcr --help"
          ;;
      esac
      exit 0
    fi
  done

  # If all installations failed
  echo "Error: Unable to install vcr to any writable location" 1>&2
  echo "Tried: $sys_exe, $HOME/.local/bin, $HOME/bin, $HOME/.vcr/bin" 1>&2
  echo "Please try one of the following:" 1>&2
  echo "  1. Run with sudo: sudo $0" 1>&2
  echo "  2. Use -o flag to specify a writable directory: $0 -o /path/to/dir" 1>&2
  echo "For more information, run: $0 -h" 1>&2
  exit 1
}
main
