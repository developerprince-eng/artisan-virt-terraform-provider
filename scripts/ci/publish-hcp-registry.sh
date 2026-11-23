#!/usr/bin/env bash
# Publishes the release in dist/ to an HCP Terraform (or Terraform Enterprise)
# private registry. Optional: runs only when TFC_TOKEN and TFC_ORGANIZATION are
# set. Users then reference app.terraform.io/<org>/artisanvirt.
#
# API reference: https://developer.hashicorp.com/terraform/cloud-docs/registry/publish-providers
#   publish-hcp-registry.sh <tag>   e.g. v1.2.3
set -euo pipefail

TAG="${1:?usage: publish-hcp-registry.sh <tag>}"
VERSION="${TAG#v}"
: "${TFC_TOKEN:?}" "${TFC_ORGANIZATION:?}" "${GPG_FINGERPRINT:?}"
HOST="${TFC_HOSTNAME:-app.terraform.io}"
ORG="${TFC_ORGANIZATION}"
NAME="${PROVIDER_NAME:?}"
API="https://${HOST}/api"

command -v jq >/dev/null 2>&1 || { apt-get update -qq && apt-get install -y -qq --no-install-recommends jq >/dev/null; }

tfc() { # tfc METHOD PATH [JSON]
  local args=(--silent --show-error --fail-with-body -X "$1" "${API}$2"
    -H "Authorization: Bearer ${TFC_TOKEN}" -H "Content-Type: application/vnd.api+json")
  [ $# -ge 3 ] && args+=(--data "$3")
  curl "${args[@]}"
}

# 1. GPG key: the registry identifies it by the 16-hex-digit key id.
FP=$(printf '%s' "$GPG_FINGERPRINT" | tr -d ' ' | tr '[:lower:]' '[:upper:]')
KEY_ID="${FP: -16}"
if tfc GET "/registry/private/v2/gpg-keys?filter%5Bnamespace%5D=${ORG}" |
   jq -e --arg k "$KEY_ID" '.data[] | select(.attributes."key-id" == $k)' >/dev/null; then
  echo "GPG key ${KEY_ID} already registered."
else
  echo "Registering GPG key ${KEY_ID}..."
  ARMOR=$(gpg --batch --armor --export "$FP")
  tfc POST "/registry/private/v2/gpg-keys" "$(jq -n --arg ns "$ORG" --arg a "$ARMOR" \
    '{data:{type:"gpg-keys",attributes:{namespace:$ns,"ascii-armor":$a}}}')" >/dev/null
fi

# 2. Provider (created once).
BASE="/v2/organizations/${ORG}/registry-providers/private/${ORG}/${NAME}"
if ! tfc GET "$BASE" >/dev/null 2>&1; then
  echo "Creating private provider ${ORG}/${NAME}..."
  tfc POST "/v2/organizations/${ORG}/registry-providers" "$(jq -n --arg n "$NAME" --arg ns "$ORG" \
    '{data:{type:"registry-providers",attributes:{name:$n,namespace:$ns,"registry-name":"private"}}}')" >/dev/null
fi

# 3. Version, then its SHA256SUMS and signature.
if tfc GET "${BASE}/versions/${VERSION}" >/dev/null 2>&1; then
  echo "Version ${VERSION} already exists in ${HOST}; nothing to do."
  exit 0
fi
PROTOCOLS=$(jq -c '.metadata.protocol_versions' terraform-registry-manifest.json)
VER=$(tfc POST "${BASE}/versions" "$(jq -n --arg v "$VERSION" --arg k "$KEY_ID" --argjson p "$PROTOCOLS" \
  '{data:{type:"registry-provider-versions",attributes:{version:$v,"key-id":$k,protocols:$p}}}')")
SUMS="dist/${PROJECT_NAME}_${VERSION}_SHA256SUMS"
curl --silent --show-error --fail -T "$SUMS" "$(jq -r '.data.links."shasums-upload"' <<<"$VER")"
curl --silent --show-error --fail -T "${SUMS}.sig" "$(jq -r '.data.links."shasums-sig-upload"' <<<"$VER")"

# 4. One platform per zip listed in SHA256SUMS.
grep '\.zip$' "$SUMS" | while read -r SHA FILE; do
  PLATFORM="${FILE#"${PROJECT_NAME}_${VERSION}_"}"; PLATFORM="${PLATFORM%.zip}"
  OS="${PLATFORM%%_*}"; ARCH="${PLATFORM#*_}"
  echo "Uploading ${OS}/${ARCH}..."
  P=$(tfc POST "${BASE}/versions/${VERSION}/platforms" "$(jq -n --arg o "$OS" --arg a "$ARCH" --arg s "$SHA" --arg f "$FILE" \
    '{data:{type:"registry-provider-version-platforms",attributes:{os:$o,arch:$a,shasum:$s,filename:$f}}}')")
  curl --silent --show-error --fail -T "dist/${FILE}" "$(jq -r '.data.links."provider-binary-upload"' <<<"$P")"
done
echo "Published ${HOST}/${ORG}/${NAME} ${VERSION}."
