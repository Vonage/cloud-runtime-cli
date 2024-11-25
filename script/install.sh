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

	if mv "$tmp_dir/${vcr_binary}" "$sys_exe"; then
    echo "vcr was installed successfully to $sys_exe"
    echo "Run 'vcr --help' to get started"
    exit 0
  else
    echo "Error encountered when moving ${vcr_binary} to $sys_exe , please try to run with sudo"
    echo "Or use the -o flag to specify a directory where you have write permissions, for more information run with -h"
    exit 1
  fi
}
main "$1"
