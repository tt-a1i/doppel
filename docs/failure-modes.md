# Failure Modes

`doppel doctor` emits findings using the stable codes below. Each finding has a severity (`info`/`warn`/`error`), a category, one or more pieces of evidence, and a short suggested fix.

## Codes

### `missing_executable` — severity: **error** — category: executable
`CFBundleExecutable` in Info.plist does not resolve to a real file under `Contents/MacOS/`. The source bundle is broken; cloning will not produce a launchable app. This is almost always an upstream issue — re-download or reinstall the source app.

### `codesign_failed` — severity: **error** — category: signature
`codesign --verify --deep --strict` reports the clone's signature is not self-consistent. The most common cause is a nested signable (framework, helper app, XPC service) that was missed during discovery or signed out of order. Re-run `doppel doctor` after a clone to see the specific failure; if doppel's discovery is missing a real signable directory, that's a bug to report.

### `sandbox_entitled` — severity: **warn** — category: sandbox
The source app declares `com.apple.security.app-sandbox`. Sandboxed apps store preferences, saved state, keychain entries, and user data in a container keyed by bundle ID. The clone has a new bundle ID, so its container is **empty** on first launch — none of the original app's data is visible. That's usually what you want for two isolated instances, but it means the clone looks "fresh" even if the source has years of data.

### `sparkle_present` — severity: **warn** — category: updater
A `Sparkle.framework` was detected. Sparkle validates updates against the app's original bundle identity and signature. Updates on the clone will either fail silently, refuse to apply, or (worst case) clobber your bundle ID back to the original's. Either disable Sparkle's auto-update in the clone's preferences, or expect to re-run `doppel` after each source-app update.

### `electron_helper` — severity: **info** — category: helper
An `Electron Framework.framework` was detected. Electron apps embed the parent bundle ID into each helper's Info.plist (e.g. `com.foo.app.helper (Renderer)`). doppel rewrites these automatically so the clone stays internally consistent. No action needed — this finding exists so you know why helper bundle IDs look different after cloning.

### `login_item_present` — severity: **warn** — category: helper
A `Contents/Library/LoginItems/*.app` was detected. Login items register with the system via `SMLoginItemSetEnabled` keyed by bundle ID. The clone's login item is unregistered on first install — if you want the clone to auto-start at login, enable it manually via **System Settings > General > Login Items**.

### `unsigned_source` — severity: **info** — category: signature
The source bundle has no code signature. This isn't an error — it just means the clone will be ad-hoc signed from scratch with no entitlements inherited. Most unsigned apps launch fine locally but will be rejected by Gatekeeper (`spctl --assess`).

## Not yet surfaced

- **Hardened runtime with strict entitlements** — certain entitlements (JIT, DYLD variables, etc.) require hardened runtime, which ad-hoc signatures can set but which Gatekeeper may still reject. No dedicated rule yet.
- **Team ID references beyond entitlements** — some apps embed team IDs in plists other than `entitlements.plist` (e.g., `SMPrivilegedExecutables` dictionary keys). Not currently scanned.
