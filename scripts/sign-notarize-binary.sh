#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "signing and notarization require macOS" >&2
  exit 3
fi

BIN="${1:-./doppel}"
OUT_ZIP="${2:-./doppel-notary.zip}"

if [[ ! -f "$BIN" ]]; then
  echo "binary not found: $BIN" >&2
  exit 2
fi

if [[ -z "${MACOS_CODESIGN_IDENTITY:-}" ]]; then
  echo "MACOS_CODESIGN_IDENTITY is required" >&2
  exit 2
fi

codesign --force --options runtime --timestamp \
  --sign "$MACOS_CODESIGN_IDENTITY" "$BIN"
codesign --verify --strict --verbose=2 "$BIN"

rm -f "$OUT_ZIP"
ditto -c -k --keepParent "$BIN" "$OUT_ZIP"

notary_args=()
if [[ -n "${MACOS_NOTARY_PROFILE:-}" ]]; then
  notary_args+=(--keychain-profile "$MACOS_NOTARY_PROFILE")
else
  if [[ -z "${MACOS_NOTARY_KEY_ID:-}" || -z "${MACOS_NOTARY_ISSUER_ID:-}" ]]; then
    echo "Set MACOS_NOTARY_PROFILE or MACOS_NOTARY_KEY_ID + MACOS_NOTARY_ISSUER_ID + MACOS_NOTARY_KEY_PATH" >&2
    exit 2
  fi
  key_path="${MACOS_NOTARY_KEY_PATH:-}"
  if [[ -z "$key_path" && -n "${MACOS_NOTARY_KEY:-}" ]]; then
    key_path="$(mktemp "${TMPDIR:-/tmp}/doppel-notary-key.XXXXXX.p8")"
    printf '%s' "$MACOS_NOTARY_KEY" | base64 --decode >"$key_path"
    trap 'rm -f "$key_path"' EXIT
  fi
  if [[ -z "$key_path" || ! -f "$key_path" ]]; then
    echo "MACOS_NOTARY_KEY_PATH must point to an App Store Connect .p8 key" >&2
    exit 2
  fi
  notary_args+=(--key "$key_path" --key-id "$MACOS_NOTARY_KEY_ID" --issuer "$MACOS_NOTARY_ISSUER_ID")
fi

xcrun notarytool submit "$OUT_ZIP" "${notary_args[@]}" --wait
spctl --assess --type execute --verbose "$BIN"

echo "signed and notarized: $BIN"
echo "notary archive: $OUT_ZIP"
