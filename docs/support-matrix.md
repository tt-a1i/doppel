# Support Matrix

Results from cloning real apps on macOS. "Cloned" means the pipeline finished without errors and `codesign --verify --deep --strict` passes on the clone. "Launches" means the clone actually opens and runs (manual verification).

| App | Type | Signables | Helpers rewritten | Entitlements | Cloned | Launches | Notes |
|---|---|---|---|---|---|---|---|
| cmux | Swift + Sparkle | 3 | 0 | yes (0 filtered) | ✅ | ✅ | Sparkle present — see `sparkle_present` finding |
| Alacritty | Rust, single-binary | 1 | 0 | none | ✅ | ✅ | Cleanest case — no frameworks |
| Ghostty | Swift + C | 3 | 0 | yes | ✅ | ✅ | Re-verified on 2026-04-18 |
| LocalSend | Flutter | 28 | 1 | yes (1 filtered) | ✅ | ✅ | Lots of Flutter plugin bundles under `Contents/Frameworks/`; helper rewrite applied |
| BetterDisplay | Swift + helpers | 4 | 0 | yes | ✅ | ⚠ | Launches, but menu-bar privileges may need re-grant on clone |
| Cherry Studio | Electron | 9 | 4 | yes | ✅ | ✅ | Verified on 2026-04-18 with source + clone running simultaneously; clone uses separate `~/Library/Application Support/Cherry Studio Clone` |
| Claude | Electron | 9 | 4 | yes | ✅ | ✗ | Clone completes and passes `codesign --verify --deep --strict`, but startup fails with `Failed to get integrity for validatable asar archive: Resources/app.asar` |
| Safari | SIP-protected Apple app | — | — | — | ✗ | — | `/Applications/Safari.app` is technically cloneable but source bundle cannot be modified; the clone target is OK, but Safari hard-codes Apple-signed expectations and won't launch |
| Google Chrome | Chrome + Helpers in Framework | — | — | — | ✗ | — | Source fails `codesign --verify --strict` due to FinderInfo xattrs. Doctor correctly flags `codesign_failed` before any clone. Not a doppel bug — Chrome ships with non-strict resources |

## Legend

- ✅ — works as documented
- ⚠ — clone completes but manual follow-up recommended (see Notes)
- ✗ — known-bad source, clone refused or produces non-working bundle

## How to reproduce

Each row above was generated with this command (TARGET is `/tmp/doppel-validate/<name>.app`):

```bash
./doppel clone /Applications/<APP_NAME>.app \
  --name <APP_NAME>-test \
  --bundle-id test.doppel.<APP_NAME> \
  --target /tmp/doppel-validate/<APP_NAME>-test.app
```

And the launch check:

```bash
open /tmp/doppel-validate/<APP_NAME>-test.app
```

## Adding a row

When you run doppel against a new app, add a row here with:

- **App name** as it appears in `/Applications`
- **Type** — rough bucket (Swift, Electron, Flutter, native C, etc.)
- **Signables count** — from the `discover` stage line
- **Helpers rewritten** — from the `plist` stage line
- **Entitlements** — presence and filter count from `entitlements` stage line
- **Cloned** / **Launches** — boolean
- **Notes** — anything surprising, plus relevant doctor codes
