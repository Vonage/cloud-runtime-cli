#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
set -e

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
      version=`echo $1 | sed -e 's/^[^=]*=//g'`
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
      path=`echo $1 | sed -e 's/^[^=]*=//g'`
      echo "path:$path"
      shift
      ;;
    *)
      echo "Error: Invalid argument $1" 1>&2
      exit 1
      ;;
  esac
done

main() {

	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)
	if [ "$arch" = "x86_64" ]; then
      arch="amd64"
  elif [ "$arch" = "aarch64" ]; then
      arch="arm64"
  fi
	vcr_binary="vcr_${os}_${arch}"
  version="${version:+download/$version}"
  version="${version:-latest/download}"

  vcr_uri="https://github.com/Vonage/cloud-runtime-cli/releases/$version/$vcr_binary.tar.gz"
	vcr_resp=$(curl -L -s -o /dev/null -w "%{http_code}" $vcr_uri)
	if [ "$vcr_resp" -ne 200 ]; then
		echo "Error: Unable to find a vcr release for vcr_${os}_${arch} - see github.com/Vonage/cloud-runtime-cli/releases for all versions" 1>&2
		exit 1
	fi

	vcr_install="${VCR_INSTALL:-$HOME/.vcr}"

	bin_dir="$vcr_install/bin"
	tmp_dir="$vcr_install/tmp"
	exe="$bin_dir/vcr"
	sys_exe="/usr/local/bin/vcr"

	mkdir -p "$bin_dir"
	mkdir -p "$tmp_dir"

  if [ -n "$path" ]; then
    echo "Path provided: $path"
    mkdir -p $path

  fi

	if ! curl -q --fail --location --progress-bar --output "$tmp_dir/${vcr_binary}.tar.gz" "$vcr_uri"; then
	  echo "Error encountered when downloading ${vcr_binary} to ${tmp_dir} , please try to run with sudo"
	  exit 1
  fi

	tar -C "$tmp_dir" -xzf "$tmp_dir/${vcr_binary}.tar.gz"
	chmod +x "$tmp_dir/${vcr_binary}"

	rm "$tmp_dir/${vcr_binary}.tar.gz"
	cp "$tmp_dir/${vcr_binary}" "$exe"

  # If custom path is provided, use it
  if [ -n "$path" ]; then
    if mv "$tmp_dir/${vcr_binary}" "$path/vcr"; then
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
	if mv "$tmp_dir/${vcr_binary}" "$sys_exe" 2>/dev/null; then
    echo "vcr was installed successfully to $sys_exe"
    echo "Run 'vcr --help' to get started"
    exit 0
  fi

  # If system-wide fails, try directories from PATH with write permissions
  for user_path in $(echo "$PATH" | tr ':' '\n'); do
    # Skip system paths and empty entries
    if [ -z "$user_path" ] || [ "$user_path" = "/usr/local/bin" ] || [ "$user_path" = "/usr/bin" ] || [ "$user_path" = "/bin" ] || [ "$user_path" = "/usr/sbin" ] || [ "$user_path" = "/sbin" ]; then
      continue
    fi

    # Try to create the directory if it doesn't exist and move the binary
    if mkdir -p "$user_path" 2>/dev/null && mv "$tmp_dir/${vcr_binary}" "$user_path/vcr" 2>/dev/null; then
      echo "vcr was installed successfully to $user_path"
      echo "Run 'vcr --help' to get started"
      exit 0
    fi
  done

  # If all installations failed, try to create and use user-local fallback directories
  fallback_paths="$HOME/.local/bin:$HOME/bin:$HOME/.vcr/bin"
  for fallback_path in $(echo "$fallback_paths" | tr ':' '\n'); do
    if mkdir -p "$fallback_path" 2>/dev/null && mv "$tmp_dir/${vcr_binary}" "$fallback_path/vcr" 2>/dev/null; then
      echo "vcr was installed successfully to $fallback_path/vcr"
      echo "Run '$fallback_path/vcr --help' to get started"

      case $SHELL in
      /bin/zsh) shell_profile=".zshrc" ;;
      *) shell_profile=".bash_profile" ;;
      esac
      echo ""
      echo "⚠️  Warning: $fallback_path is not in your \$PATH"
      echo "Add the following to your \$HOME/$shell_profile:"
      echo "  export PATH=\"$fallback_path:\$PATH\""
      exit 0
    fi
  done

  # If all installations failed
  echo "Error: Unable to install vcr to any writable location" 1>&2
  echo "Tried: $sys_exe and all writable directories in \$PATH" 1>&2
  echo "Please try one of the following:" 1>&2
  echo "  1. Run with sudo: sudo $0" 1>&2
  echo "  2. Use -o flag to specify a writable directory: $0 -o /path/to/dir" 1>&2
  echo "For more information, run: $0 -h" 1>&2
  exit 1
}
main "$1"
