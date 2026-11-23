#!/usr/bin/env bash
# Imports the release signing key into the job's GnuPG home.
#
# GPG_PRIVATE_KEY may be a GitLab "File" variable (a path), an ASCII-armored
# key, or that key base64-encoded (convenient for masking). GPG_FINGERPRINT
# selects the key GoReleaser signs with; GPG_PASSPHRASE is optional.
set -euo pipefail

: "${GPG_PRIVATE_KEY:?GPG_PRIVATE_KEY is not set (CI/CD variable, protected)}"
: "${GPG_FINGERPRINT:?GPG_FINGERPRINT is not set}"

export GNUPGHOME="${GNUPGHOME:-$HOME/.gnupg}"
mkdir -p "$GNUPGHOME" && chmod 700 "$GNUPGHOME"
echo "allow-loopback-pinentry" >> "$GNUPGHOME/gpg-agent.conf"

key=$(mktemp)
trap 'rm -f "$key"' EXIT
if [ -f "$GPG_PRIVATE_KEY" ]; then
  cp "$GPG_PRIVATE_KEY" "$key"
elif printf '%s' "$GPG_PRIVATE_KEY" | grep -q -- '-----BEGIN PGP PRIVATE KEY BLOCK-----'; then
  printf '%s\n' "$GPG_PRIVATE_KEY" > "$key"
else
  printf '%s' "$GPG_PRIVATE_KEY" | base64 -d > "$key"
fi

gpg --batch --import "$key"
fp=$(printf '%s' "$GPG_FINGERPRINT" | tr -d ' ' | tr '[:lower:]' '[:upper:]')
if ! gpg --batch --list-secret-keys --with-colons | grep -q "^fpr:::::::::${fp}:"; then
  echo "No secret key with fingerprint ${fp} was imported." >&2
  gpg --batch --list-secret-keys --keyid-format long >&2
  exit 1
fi
echo "Imported signing key ${fp}."
