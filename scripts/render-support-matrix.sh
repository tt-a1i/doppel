#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 2
fi

if [[ $# -eq 0 ]]; then
  set -- /tmp/doppel-smoke/*.json
fi

printf '| App | Type | Signables | Helpers rewritten | Entitlements | Cloned | Launches | Notes |\n'
printf '|---|---|---|---|---|---|---|---|\n'

for report in "$@"; do
  [[ -f "$report" ]] || continue

  source_app="$(jq -r '.source_app // .report.app_path // ""' "$report")"
  app="$(basename "$source_app" .app)"
  [[ -n "$app" && "$app" != "." ]] || app="$(basename "$report" .json)"

  success="$(jq -r '.success // false' "$report")"
  cloned="✗"
  if [[ "$success" == "true" ]]; then
    cloned="✅"
  fi

  launch_attempted="$(jq -r '.verify.launch_test.attempted // .report.launch_test.attempted // false' "$report")"
  launch_survived="$(jq -r '.verify.launch_test.survived // .report.launch_test.survived // false' "$report")"
  launches="⚠"
  if [[ "$launch_attempted" == "true" && "$launch_survived" == "true" ]]; then
    launches="✅"
  elif [[ "$launch_attempted" == "true" ]]; then
    launches="✗"
  fi

  discover_msg="$(jq -r '.stages[]? | select(.stage == "discover" and (.status == "ok" or .status == "warn")) | .message' "$report" | tail -n1)"
  signables="—"
  if [[ "$discover_msg" =~ ^([0-9]+)[[:space:]]found ]]; then
    signables="${BASH_REMATCH[1]}"
  fi

  helpers="$(jq -r '(.helper_rewrites // []) | length' "$report")"
  ent_count="$(jq -r '(.entitlement_changes // []) | length' "$report")"
  entitlements="none"
  if [[ "$ent_count" -gt 0 ]]; then
    entitlements="yes (${ent_count} changes)"
  fi

  codes="$(jq -r '(.preflight_findings // []) | map(.code) | join(", ")' "$report")"
  note="Generated from $(basename "$report")"
  if [[ -n "$codes" ]]; then
    note="$note; doctor: $codes"
  fi

  printf '| %s | unknown | %s | %s | %s | %s | %s | %s |\n' \
    "$app" "$signables" "$helpers" "$entitlements" "$cloned" "$launches" "$note"
done
