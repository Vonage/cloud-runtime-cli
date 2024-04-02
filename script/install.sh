#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
set -e

main() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)
	if [ "$arch" = "x86_64" ]; then
      arch="amd64"
  fi
	vcr_binary="vcr_${os}_${arch}"
  version="${1:+download/$1}"
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

	if ! curl -q --fail --location --progress-bar --output "$tmp_dir/${vcr_binary}.tar.gz" "$vcr_uri"; then
	  echo "Error encountered when downloading ${vcr_binary} to ${tmp_dir} , please try to run with sudo"
	  exit 1
  fi

	tar -C "$tmp_dir" -xzf "$tmp_dir/${vcr_binary}.tar.gz"
	chmod +x "$tmp_dir/${vcr_binary}"

	rm "$tmp_dir/${vcr_binary}.tar.gz"
	cp "$tmp_dir/${vcr_binary}" "$exe"
	if mv "$tmp_dir/${vcr_binary}" "$sys_exe" 2>/dev/null; then
     echo "vcr was installed successfully to $sys_exe"
     echo "Run 'vcr --help' to get started"
  else
    case $SHELL in
    /bin/zsh) shell_profile=".zshrc" ;;
    *) shell_profile=".bash_profile" ;;
    esac
    echo "Error encountered when moving ${vcr_binary} to $sys_exe , please try to run with sudo"
    echo "Or manually add the directory to your \$HOME/$shell_profile (or similar)"
    echo "  export VCR_INSTALL=\"$vcr_install\""
    echo "  export PATH=\"\$VCR_INSTALL/bin:\$PATH\""
    echo "Run '$exe --help' to get started"
  fi
}

main "$1"