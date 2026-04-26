#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${DOPPEL_BIN:-$ROOT/doppel}"
OUT_DIR="${DOPPEL_SMOKE_DIR:-/tmp/doppel-smoke}"
RUN_LAUNCH_TEST="${DOPPEL_SMOKE_LAUNCH_TEST:-1}"

default_apps=(
  "/Applications/cmux.app"
  "/Applications/Alacritty.app"
  "/Applications/Ghostty.app"
  "/Applications/LocalSend.app"
  "/Applications/Cherry Studio.app"
)

apps=("$@")
if [[ ${#apps[@]} -eq 0 ]]; then
  apps=("${default_apps[@]}")
fi

slugify() {
  local s="$1"
  s="${s%".app"}"
  s="$(tr '[:upper:]' '[:lower:]' <<<"$s")"
  s="$(sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//' <<<"$s")"
  if [[ -z "$s" ]]; then
    s="app"
  fi
  printf '%s' "$s"
}

clone_one() {
  local app="$1"
  local base slug name bid target report args

  if [[ ! -d "$app" ]]; then
    printf '| `%s` | skipped | source app missing |\n' "$app"
    return 0
  fi

  base="$(basename "$app")"
  slug="$(slugify "$base")"
  name="${base%.app} Doppel Smoke"
  bid="test.doppel.${slug}"
  target="$OUT_DIR/${name}.app"
  report="$OUT_DIR/${slug}.json"

  args=(
    clone "$app"
    --name "$name"
    --bundle-id "$bid"
    --target "$target"
    --force
    --json
  )
  if [[ "$RUN_LAUNCH_TEST" == "1" ]]; then
    args+=(--launch-test)
  fi

  if ! "$BIN" "${args[@]}" >"$report"; then
    printf '| `%s` | fail | `%s` |\n' "$base" "$report"
    return 0
  fi

  if [[ "$RUN_LAUNCH_TEST" == "1" ]]; then
    if ! jq -e '.verify.launch_test.survived == true' "$report" >/dev/null; then
      printf '| `%s` | fail | `%s` |\n' "$base" "$report"
      return 0
    fi
  fi

  printf '| `%s` | pass | `%s` |\n' "$base" "$report"
}

main() {
	mkdir -p "$OUT_DIR"
	if [[ ! -x "$BIN" ]]; then
	    (cd "$ROOT" && go build -o "$BIN" ./cmd/doppel)
	fi
	if [[ "$RUN_LAUNCH_TEST" == "1" ]] && ! command -v jq >/dev/null 2>&1; then
	  echo "jq is required when DOPPEL_SMOKE_LAUNCH_TEST=1" >&2
	  exit 2
	fi

	printf '# doppel real-app smoke\n\n'
  printf -- '- Binary: `%s`\n' "$BIN"
  printf -- '- Output: `%s`\n' "$OUT_DIR"
  printf -- '- Launch test: `%s`\n\n' "$RUN_LAUNCH_TEST"
  printf '| App | Result | Report |\n'
  printf '|---|---|---|\n'

  for app in "${apps[@]}"; do
    clone_one "$app"
  done
}

main
