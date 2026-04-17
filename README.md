# appclone

A macOS-only tool to clone a `.app` bundle into a second, separately-launchable app instance with a new bundle identifier and local ad-hoc re-signing.

One binary, two modes:

- **TUI** (default): `appclone` — full-screen interactive app picker → config form → live progress → result page
- **CLI**: `appclone <inspect|clone|verify|doctor> …` — scriptable, `--json` output

## Install

```bash
git clone https://github.com/tt-a1i/appclone
cd appclone
make build         # produces ./appclone
make install       # copies to $GOPATH/bin (optional)
```

Requires macOS and Go 1.21+. At runtime, shells out to `/usr/bin/ditto`, `/usr/bin/codesign`, and `/usr/sbin/spctl`, all of which are standard on macOS.

## Usage

### Interactive (TUI)

```bash
appclone
```

Launches a full-screen picker that scans `/Applications`, `/Applications/Utilities`, and `~/Applications`. Pick an app, fill in the new name / bundle ID, watch the clone pipeline run.

### CLI

```bash
# Enumerate installable apps (agent-friendly)
appclone list
appclone list --json

# Inspect a specific bundle
appclone inspect /Applications/cmux.app
appclone inspect /Applications/cmux.app --json

# Dry-run (derive plan, touch nothing)
appclone clone /Applications/cmux.app --name cmux2 --bundle-id com.example.cmux2 --dry-run

# Real clone
appclone clone /Applications/cmux.app \
  --name cmux2 \
  --bundle-id com.example.cmux2 \
  --target /Applications/cmux2.app

# Re-clone over an existing target (deletes target first; target must end in .app)
appclone clone /Applications/cmux.app --name cmux2 --bundle-id com.example.cmux2 --force

appclone verify /Applications/cmux2.app
appclone doctor /Applications/cmux.app
```

Global flags: `--json` (structured output), `--verbose`.
Run `appclone <cmd> --help` for per-command flag lists.

### Exit codes (for scripting)

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | general error |
| 2 | invalid input (bad path, bad bundle id, target exists without `--force`, etc.) |
| 3 | unsupported environment (not macOS) |
| 4–9 | stage-specific failures: copy / plist / sign / verify / launch-test / inspect |

## How It Works

The clone pipeline has six stages, each emitted as a stage event for the TUI / CLI to render:

1. **copy** — `/usr/bin/ditto` preserves xattrs, ACLs, and resource forks that `cp` can't
2. **plist** — rewrites `CFBundleIdentifier`, `CFBundleName`, `CFBundleDisplayName`; also rewrites helper bundle IDs that embed the parent ID (Electron pattern)
3. **entitlements** — extracts from source, strips identity-bound keys (`application-identifier`, `keychain-access-groups`, team-identifier, etc.) that would fail after an ad-hoc re-sign
4. **discover** — walks `Contents/Frameworks`, `XPCServices`, `PlugIns`, `Helpers`, `LoginItems` for nested signables, sorted deepest-first
5. **resign** — ad-hoc (`codesign --sign -`) in post-order: deepest items first, outer bundle last
6. **verify** — `codesign --verify --deep --strict` + optional `spctl --assess`

Known trade-offs:

- **Ad-hoc signing is local-launchable, not vendor-trust-valid.** `spctl` will reject the clone; Gatekeeper will prompt on first launch. That's normal — local ad-hoc signatures are what Apple wants to see for local dev builds, not distribution.
- **Never modifies the source bundle.** Source app is treated read-only end to end.
- **No `--force`** — if the target path already exists, appclone refuses and tells you. You can decide whether to delete the existing clone manually.

## What's Supported

See [`docs/support-matrix.md`](docs/support-matrix.md) for per-app results and [`docs/failure-modes.md`](docs/failure-modes.md) for the doctor rule catalog.

TL;DR: Swift/Rust/native apps clone very cleanly. Electron clones structurally but embeds parent bundle IDs in helpers — appclone rewrites those automatically. Sparkle-updated apps clone fine but the updater will break. Sandboxed apps clone fine but start with empty containers. Apps shipped with `codesign --strict` issues on disk (e.g., Chrome's FinderInfo xattrs) are flagged before any clone runs.

## Caveats

- macOS only (enforced at startup)
- Experimental — not all apps will clone cleanly; `doctor` surfaces the common failure patterns
- App Store apps are out of scope for v1
- Does not preserve vendor notarization, update channels, or entitlement identities tied to the original team
