#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${DOPPEL_BIN:-$ROOT/doppel}"
OUT_DIR="${DOPPEL_FIXTURE_SMOKE_DIR:-/tmp/doppel-fixture-smoke}"
SRC_APP="$OUT_DIR/DoppelFixture.app"
TARGET_APP="$OUT_DIR/DoppelFixtureClone.app"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "fixture smoke requires macOS" >&2
  exit 3
fi

rm -rf "$OUT_DIR"
mkdir -p "$SRC_APP/Contents/MacOS"

cat >"$SRC_APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key><string>en</string>
  <key>CFBundleExecutable</key><string>DoppelFixture</string>
  <key>CFBundleIdentifier</key><string>test.doppel.fixture</string>
  <key>CFBundleName</key><string>DoppelFixture</string>
  <key>CFBundleDisplayName</key><string>DoppelFixture</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundleVersion</key><string>1</string>
</dict>
</plist>
PLIST

cat >"$SRC_APP/Contents/MacOS/DoppelFixture" <<'SH'
#!/bin/sh
sleep 60
SH
chmod +x "$SRC_APP/Contents/MacOS/DoppelFixture"

codesign --force --sign - "$SRC_APP" >/dev/null

if [[ ! -x "$BIN" ]]; then
  (cd "$ROOT" && go build -o "$BIN" ./cmd/doppel)
fi

"$BIN" doctor "$SRC_APP" --json >"$OUT_DIR/doctor.json"
"$BIN" clone "$SRC_APP" \
  --name DoppelFixtureClone \
  --target "$TARGET_APP" \
  --dry-run \
  --json >"$OUT_DIR/dry-run.json"
"$BIN" clone "$SRC_APP" \
  --name DoppelFixtureClone \
  --target "$TARGET_APP" \
  --launch-test \
  --json >"$OUT_DIR/clone.json"
"$BIN" verify "$TARGET_APP" --json >"$OUT_DIR/verify.json"

echo "fixture smoke passed: $OUT_DIR"
