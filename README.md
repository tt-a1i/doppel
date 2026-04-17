# appclone

macOS-only tool to clone a `.app` bundle into a second, separately-launchable app instance with a new bundle identifier and local ad-hoc re-signing.

Ships as a single binary with two modes:

- **TUI** (default): `appclone` — full-screen interactive picker / cloner
- **CLI**: `appclone <inspect|clone|verify|doctor> …` — scriptable, `--json` output

## Status

Early development. Skeleton only.

## Build

```bash
make build      # -> ./appclone
make run        # build + launch TUI
```

## Usage

```bash
appclone                                      # launch TUI
appclone inspect /Applications/cmux.app
appclone clone   /Applications/cmux.app --name cmux2 --bundle-id com.cmuxterm.app2
appclone verify  /Applications/cmux2.app
appclone doctor  /Applications/cmux2.app
```

## Caveats

- macOS only
- Ad-hoc re-signing produces a **locally launchable** app, not a vendor-trust-chain-valid one
- Experimental — not all apps will clone cleanly (Sparkle updaters, App Store/sandboxed apps, tightly entitled helpers are known rough spots)
- Never modifies the source `.app`
