#!/usr/bin/env bash
# Installs pinned CLI tools into /usr/local/bin, verifying published checksums.
#   install-tools.sh terraform goreleaser
# Versions come from TERRAFORM_VERSION and GORELEASER_VERSION (.gitlab-ci.yml).
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1; }
if ! need curl || ! need unzip; then
  apt-get update -qq && apt-get install -y -qq --no-install-recommends curl unzip ca-certificates >/dev/null
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

install_terraform() {
  local v="${TERRAFORM_VERSION:?TERRAFORM_VERSION is not set}"
  local zip="terraform_${v}_linux_amd64.zip"
  local base="https://releases.hashicorp.com/terraform/${v}"
  echo "Installing terraform ${v}..."
  curl -fsSLo "$tmp/$zip" "$base/$zip"
  curl -fsSLo "$tmp/SHA256SUMS" "$base/terraform_${v}_SHA256SUMS"
  (cd "$tmp" && grep " ${zip}\$" SHA256SUMS | sha256sum -c -)
  unzip -oq "$tmp/$zip" terraform -d /usr/local/bin
  terraform -version
}

install_goreleaser() {
  local v="${GORELEASER_VERSION:?GORELEASER_VERSION is not set}"
  local tgz="goreleaser_Linux_x86_64.tar.gz"
  local base="https://github.com/goreleaser/goreleaser/releases/download/${v}"
  echo "Installing goreleaser ${v}..."
  curl -fsSLo "$tmp/$tgz" "$base/$tgz"
  curl -fsSLo "$tmp/checksums.txt" "$base/checksums.txt"
  (cd "$tmp" && grep "  ${tgz}\$" checksums.txt | sha256sum -c -)
  tar -xzf "$tmp/$tgz" -C /usr/local/bin goreleaser
  goreleaser --version | head -n 3
}

for tool in "$@"; do
  case "$tool" in
    terraform) install_terraform ;;
    goreleaser) install_goreleaser ;;
    *) echo "unknown tool: $tool" >&2; exit 2 ;;
  esac
done
