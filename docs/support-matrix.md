# Support Matrix

Results from cloning real apps on macOS. "Cloned" means the clone pipeline and `codesign --verify --deep --strict` pass. "Launches" means `--launch-test` reports the cloned app survived its startup window, or a manual launch was explicitly noted.

| App | Type | Signables | Helpers rewritten | Entitlements | Cloned | Launches | Notes |
|---|---|---|---|---|---|---|---|
| cmux | Swift + Sparkle | 4 | 1 | yes (1 change) | ✅ | ✅ | Re-verified with `--launch-test` on 2026-04-26; Sparkle present; dock tile plugin bundle ID rewritten |
| Alacritty | Rust, single-binary | 1 | 0 | none | ✅ | ✅ | Re-verified with `--launch-test` on 2026-04-26; cleanest case — no frameworks |
| Ghostty | Swift + C | 3 | 0 | yes (1 change) | ✅ | ✅ | Re-verified with `--launch-test` on 2026-04-26; Sparkle present |
| LocalSend | Flutter | 28 | 1 | yes (2 changes) | ✅ | ✗ | Clone verifies, but 2026-04-26 `--launch-test` exits after ~2.5s with no crash report; sandbox + login item findings |
| BetterDisplay | Swift + helpers | 4 | 0 | yes | ✅ | ⚠ | Launches, but menu-bar privileges may need re-grant on clone |
| Cherry Studio | Electron | 9 | 4 | yes (1 change) | ✅ | ✅ | Re-verified with `--launch-test` on 2026-04-26; strips `ElectronAsarIntegrity`; clone uses separate app support directory |
| Claude | Electron | 9 | 4 | yes | ✅ | ✗ | Clone completes and passes `codesign --verify --deep --strict`, but startup fails with `Failed to get integrity for validatable asar archive: Resources/app.asar` |
| Safari | SIP-protected Apple app | — | — | — | ✗ | — | `/Applications/Safari.app` is technically cloneable but source bundle cannot be modified; the clone target is OK, but Safari hard-codes Apple-signed expectations and won't launch |
| Google Chrome | Chrome + Helpers in Framework | — | — | — | ✗ | — | Source fails `codesign --verify --strict` due to FinderInfo xattrs. Doctor correctly flags `codesign_failed` before any clone. Not a doppel bug — Chrome ships with non-strict resources |

## Legend

- ✅ — works as documented
- ⚠ — clone completes but manual follow-up recommended (see Notes)
- ✗ — known-bad source, clone refused or produces non-working bundle

## How to reproduce

Use the smoke harness before releases:

```bash
scripts/smoke-real-apps.sh
```

To run clone/verify without launching apps:

```bash
DOPPEL_SMOKE_LAUNCH_TEST=0 scripts/smoke-real-apps.sh
```

The underlying command shape is:

```bash
./doppel clone /Applications/<APP_NAME>.app \
  --name <APP_NAME>-test \
  --bundle-id test.doppel.<APP_NAME> \
  --target /tmp/doppel-validate/<APP_NAME>-test.app \
  --force \
  --launch-test \
  --json
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
