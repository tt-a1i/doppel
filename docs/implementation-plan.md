# Doppel Implementation Plan

> Working plan for building Doppel. Each task has objectives, file list, acceptance criteria, and tests. TDD where it makes sense: write failing test → implement → pass → commit.

## 1. Product

**Goal:** A macOS-only tool that clones an installed `.app` bundle into a second, separately-launchable app instance — new bundle ID, new display name, locally re-signed ad-hoc, usable immediately.

**Two interfaces, one core:**
- **TUI** (default, no arguments): full-screen interactive picker → form → progress → result
- **CLI** (subcommands): `inspect`, `clone`, `verify`, `doctor` — scriptable, with `--json` output

**Non-goals (v1):**
- Preserving vendor notarization or update channels
- Full sandbox/keychain/container isolation
- App Store apps
- Any mutation of the source bundle

**Success criteria:** clone 3–5 real-world apps cleanly; clear diagnostics for the others; deterministic exit codes; no silent destructive behavior.

## 2. Tech Stack

| Concern | Choice | Why |
|---|---|---|
| Language | Go 1.26+ | Single binary, fast, solid `os/exec` |
| TUI | `charmbracelet/bubbletea` + `lipgloss` + `bubbles` | Elm-style, great for stage-based progress + forms |
| CLI | `spf13/cobra` | Standard for Go CLIs, good help/completion |
| Plist | `howett.net/plist` | Handles binary + XML plists natively |
| Copy | Shell out to `/usr/bin/ditto` | Preserves xattrs, ACLs, resource forks (issues `cp`/`io.Copy` can't solve) |
| Sign | Shell out to `codesign` | No Go equivalent |
| Tests | stdlib `testing` + table-driven | No framework tax |

## 3. Repo Structure

```
doppel/
├── cmd/doppel/main.go          # Entry: args → CLI, no args → TUI
├── internal/
│   ├── core/                     # Pure logic, no UI deps. TUI + CLI both consume.
│   │   ├── appinfo/              # Inspect bundle → structured model
│   │   ├── plistops/             # Read / mutate Info.plist
│   │   ├── signing/              # Nested discovery + entitlements + resign
│   │   ├── clone/                # Pipeline orchestration, emits stage events
│   │   ├── verify/               # codesign verify + spctl + plist consistency
│   │   ├── doctor/               # Rule engine: evidence → category/severity/fix
│   │   ├── macos/                # Thin wrappers: codesign/plutil/spctl/ditto/open
│   │   ├── errors/               # Typed errors
│   │   └── exitcodes/            # Exit code constants
│   ├── tui/                      # Bubble Tea program
│   │   ├── app.go                # Root model, screen router
│   │   ├── applist/              # Scan /Applications, pick one
│   │   ├── form/                 # New name / bundle id / target path
│   │   ├── progress/             # 7-stage live progress
│   │   └── result/               # Success / warnings / failure page
│   └── cli/                      # Cobra subcommands. Renders core output as text or JSON.
├── docs/
│   ├── implementation-plan.md    # This file
│   ├── support-matrix.md         # Which apps work, which don't
│   └── failure-modes.md          # doctor rule catalog
├── testdata/                     # Fake .app bundles built in-test or stored here
├── Makefile
├── go.mod / go.sum
└── README.md
```

**Dependency rule:** `internal/core/*` must not import `internal/tui` or `internal/cli`. Core emits events through channels; UI layers consume.

## 4. Clone Pipeline — Stages

Clone is modeled as an ordered sequence of stages. Each stage emits a `StageEvent` (start / progress / done / warn / error) on a channel. TUI renders progress; CLI prints lines or serializes to JSON.

1. **Inspect** source bundle (read Info.plist, detect signables, signature status)
2. **Plan** — derive target path, target bundle ID, items to re-sign
3. **Copy** — `ditto` source → target
4. **Mutate plist** — update `CFBundleIdentifier`, `CFBundleName`, `CFBundleDisplayName`
5. **Entitlements** — extract source entitlements; strip conflicts (e.g. `application-identifier`, `keychain-access-groups`)
6. **Re-sign** — deep, post-order (nested signables first, outer bundle last)
7. **Verify** — `codesign --verify --deep --strict`; optional `spctl --assess`; optional launch test

## 5. Output & Exit Codes

### Human output

Stage-based live output in both TUI and CLI text mode:

```
[1/7] Inspecting source app…        ✓
[2/7] Planning clone…               ✓
[3/7] Copying bundle…               ✓
[4/7] Updating Info.plist…          ✓
[5/7] Extracting entitlements…      ⚠ stripped keychain-access-groups
[6/7] Re-signing (14 items)…        ✓
[7/7] Verifying signature…          ✓

SUCCESS → /Applications/cmux2.app
Warnings:
  • Sparkle updater detected — auto-update likely broken on clone
  • Helper bundle IDs were rewritten to match parent (Electron pattern)
```

### JSON output (`--json`)

Stable schema:

```json
{
  "success": true,
  "command": "clone",
  "source_app": "/Applications/cmux.app",
  "target_app": "/Applications/cmux2.app",
  "bundle_id_before": "com.cmuxterm.app",
  "bundle_id_after": "com.cmuxterm.app2",
  "stages": [{"name": "copy", "status": "ok", "duration_ms": 312}, …],
  "detected_components": {"frameworks": […], "helpers": […], …},
  "signature": {"verified": true, "deep": true, "strict": true, "spctl": "rejected"},
  "launch_test": {"attempted": false},
  "warnings": [{"code": "sparkle_present", "message": "…"}],
  "errors": []
}
```

### Exit codes (`internal/core/exitcodes`)

```go
const (
    OK                       = 0
    GeneralError             = 1
    InvalidInput             = 2
    UnsupportedEnvironment   = 3
    CopyFailed               = 4
    PlistMutationFailed      = 5
    SigningFailed            = 6
    VerificationFailed       = 7
    LaunchTestFailed         = 8
    InspectionFailed         = 9
)
```

## 6. Guardrails

1. **Never mutate the source bundle.** Every function that takes a source path treats it read-only.
2. **Never overwrite an existing target** in v1. No `--force` flag yet.
3. **"Locally launchable" ≠ "vendor-trust valid."** Surface this distinction everywhere. `spctl` may reject an ad-hoc-signed clone that launches fine locally — that's expected.
4. **Core stays GUI-ready.** No `fmt.Println` in `internal/core/*`. Results are structs; events go through channels.
5. **Diagnostics are product, not afterthought.** `doctor` output quality is a primary success criterion.

---

# Task Breakdown

Each task: objective → files → acceptance criteria → tests → commit message. Work top-down; don't skip to signing before the core model is stable.

## Task 1 — Skeleton ✅

**Done.** Module initialized; `cmd/doppel/main.go` dispatches TUI vs CLI; Cobra subcommand stubs return "not implemented"; Bubble Tea hello model; Makefile; `./doppel --help` works.

Commit: `c774763 feat: initialize appclone skeleton with go module and tui/cli dispatch` (pre-rename)

## Task 2 — Environment & path validation + error types

**Objective:** Fail early with clear messages on non-macOS, missing app path, non-`.app` path.

**Files:**
- `internal/core/errors/errors.go` — typed errors: `ErrUnsupportedOS`, `ErrNotAnApp`, `ErrAppMissing`, `ErrAppUnreadable`
- `internal/core/exitcodes/exitcodes.go` — the constants above
- `internal/core/appinfo/validate.go` — `RequireMacOS()`, `ValidateAppPath(p string) error`
- `internal/cli/cli.go` — map errors → exit codes
- `internal/core/appinfo/validate_test.go` — table-driven tests

**Acceptance criteria:**
- Non-macOS call of `RequireMacOS()` returns `ErrUnsupportedOS`.
- `ValidateAppPath` rejects: non-existent, file (not dir), directory without `Contents/Info.plist`, non-`.app` suffix.
- CLI errors print one clear line to stderr and exit with the matching exit code (not bare `1`).

**Tests:**
- Build fake bundles in `t.TempDir()`: valid `.app` dir, `.app` without `Contents`, non-`.app` dir, nonexistent path.
- Assert error type via `errors.Is`.

**Commit:** `feat(core): add platform and app path validation`

## Task 3 — App inspection model

**Objective:** Read `Info.plist` into structured models; detect whether a signature is present.

**Files:**
- `internal/core/appinfo/model.go` — `AppIdentity`, `InspectionReport`
- `internal/core/appinfo/inspect.go` — `Inspect(app string) (*InspectionReport, error)`
- `internal/core/appinfo/inspect_test.go`

**Model:**
```go
type AppIdentity struct {
    AppPath       string
    BundleID      string
    BundleName    string
    DisplayName   string
    ExecutableName string
    Version       string
    Build         string
}

type InspectionReport struct {
    Identity    AppIdentity
    HasSignature bool   // presence of _CodeSignature/CodeResources
    Entitled    bool   // entitlements embedded in binary (codesign -d --entitlements)
    Executable  string // resolved absolute path
}
```

**Acceptance:**
- Reads both XML and binary plists (library handles this).
- Missing optional fields return zero values, not errors.
- `HasSignature` = true iff `Contents/_CodeSignature/CodeResources` exists.

**Tests:**
- Build fake bundles with plist fixtures (XML + binary).
- Missing `CFBundleDisplayName` → empty string.
- Nonexistent executable → error at resolution, not during plist parse.

**Commit:** `feat(core): add app inspection model`

## Task 4 — macOS tool wrappers

**Objective:** Centralize all shell-outs. Make them unit-testable by taking an `Execer` interface.

**Files:**
- `internal/core/macos/exec.go` — `Execer` interface, `RealExecer`, `FakeExecer` (test helper)
- `internal/core/macos/codesign.go` — `Verify`, `Sign`, `ExtractEntitlements`
- `internal/core/macos/plutil.go` — `Convert`, `Lint`
- `internal/core/macos/spctl.go` — `Assess`
- `internal/core/macos/ditto.go` — `Copy`
- `internal/core/macos/open.go` — `LaunchTest`
- `internal/core/macos/macos_test.go`

**Design:**
```go
type Execer interface {
    Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}
```

Every high-level function takes an `Execer`, not `exec.Command` directly. Tests inject `FakeExecer` with scripted responses.

**Acceptance:**
- All wrappers return typed results (e.g. `CodesignVerifyResult { OK bool; Stderr string }`) not raw `*exec.Cmd`.
- Timeouts are caller-controlled via `context.Context`.
- Non-zero exit is not always an error (e.g. `spctl --assess` rejection is a result, not an error).

**Tests:** `FakeExecer` returning scripted output for each wrapper; assert parsing.

**Commit:** `feat(core): add macos tool wrappers with injectable exec`

## Task 5 — Plist mutation

**Objective:** Safely update clone identity fields.

**Files:**
- `internal/core/plistops/plistops.go` — `ReadInfoPlist`, `WriteInfoPlist`, `SetIdentity`
- `internal/core/plistops/plistops_test.go`

**API:**
```go
type InfoPlist map[string]any

func ReadInfoPlist(appPath string) (InfoPlist, error)
func WriteInfoPlist(appPath string, p InfoPlist) error
func SetIdentity(p InfoPlist, bundleID, name, displayName string) InfoPlist
```

**Acceptance:**
- Unrelated keys preserved bit-for-bit (read roundtrip test).
- Works on both XML and binary plists; writes back in original format.
- Missing `CFBundleDisplayName` → added; existing → updated.

**Tests:** roundtrip on binary + XML plists; identity-set preserves sibling keys.

**Commit:** `feat(core): add plist mutation helpers`

## Task 6 — Nested signable discovery

**Objective:** Walk a bundle and return all signable paths in **post-order** (deepest first, main bundle last).

**Files:**
- `internal/core/signing/discover.go` — `Discover(appPath string) ([]SignableItem, error)`
- `internal/core/signing/discover_test.go`

**Model:**
```go
type SignableKind int
const (
    KindFramework SignableKind = iota
    KindHelperApp
    KindXPCService
    KindPlugin
    KindLoginItem
    KindMainExecutable
    KindMainBundle
)

type SignableItem struct {
    Path string
    Kind SignableKind
    Depth int // deeper = sign first
}
```

**Paths to scan:**
- `Contents/Frameworks/*.framework`
- `Contents/Helpers/*.app` (also scan nested `Contents/Frameworks/` recursively)
- `Contents/XPCServices/*.xpc`
- `Contents/PlugIns/*.bundle`
- `Contents/Library/LoginItems/*.app`
- `Contents/MacOS/<executable>` (from Info.plist)
- The bundle root itself (last)

**Acceptance:**
- Returned slice is sorted by depth DESC then path ASC (deterministic).
- Nested frameworks inside helper apps are found (recursive).
- Missing optional directories aren't errors.

**Tests:** build synthetic bundles with each kind in `t.TempDir()`; assert order + count.

**Commit:** `feat(core): discover nested signable items in bundle order`

## Task 7 — Clone plan + bundle copy

**Objective:** Derive a deterministic plan from inputs; copy bundle with `ditto`.

**Files:**
- `internal/core/clone/plan.go` — `DerivePlan(opts)`, `ClonePlan`
- `internal/core/clone/copy.go` — `CopyBundle(ctx, plan, execer)`
- `internal/core/clone/plan_test.go`
- `internal/core/clone/copy_test.go`

**Model:**
```go
type PlanOptions struct {
    SourceApp   string
    Name        string   // required, used to derive target path + bundle id fallbacks
    TargetApp   string   // optional, defaults to ~/Applications/<Name>.app
    BundleID    string   // required
    DisplayName string   // optional, defaults to Name
    DryRun      bool
}

type ClonePlan struct {
    SourceApp       string
    TargetApp       string
    BundleIDBefore  string
    BundleIDAfter   string
    NameAfter       string
    DisplayNameAfter string
    DryRun          bool
}
```

**Helper bundle ID handling:** For each helper app under `Contents/Helpers/*.app` whose original bundle ID has the source bundle ID as a **prefix** (Electron pattern: `com.foo.app.helper`), rewrite its bundle ID by substituting the prefix. Store these rewrites in the plan so Task 8 can apply them before signing.

**Acceptance:**
- Default target: `~/Applications/<Name>.app` when `--target` omitted.
- Refuses if target == source.
- Refuses if target already exists (v1 — no `--force`).
- `--dry-run` produces complete plan, touches nothing.
- `CopyBundle` uses `ditto` (not `cp -r` — preserves xattrs/ACLs).

**Tests:** plan derivation table tests; `CopyBundle` with `FakeExecer` asserts correct `ditto` invocation.

**Commit:** `feat(core): derive clone plan and copy via ditto`

## Task 8 — Entitlements + deep re-sign

**Objective:** Extract source entitlements, strip conflicting keys, re-sign post-order.

**Files:**
- `internal/core/signing/entitlements.go` — extract + filter
- `internal/core/signing/resign.go` — `DeepResign(ctx, plan, items, entitlements, execer)`
- `internal/core/signing/*_test.go`

**Entitlement filter (strip when bundle ID changes):**
- `application-identifier`
- `com.apple.application-identifier`
- `keychain-access-groups`
- `com.apple.developer.team-identifier`
- `com.apple.developer.associated-domains` (optional — often tied to identifier)
- Any entitlement whose value contains the **source** bundle ID literal → rewrite to new ID

**Signing:**
- Ad-hoc: `codesign --force --sign - --timestamp=none`
- Entitlements pass via `--entitlements <plist>`
- Deep handled by us via post-order iteration, not `--deep` (more control)

**Acceptance:**
- Nested items signed strictly before their parents.
- Main bundle signed last.
- Missing entitlements (app was never signed) → ad-hoc sign with no entitlements flag, succeeds.
- Any signing failure aborts with typed error carrying the failed path.

**Tests:** `FakeExecer` scripting multi-step sign; assert command order via recorded calls; entitlement-filter table tests.

**Commit:** `feat(core): extract entitlements and deep post-order resign`

## Task 9 — Verification

**Objective:** Verify clone structurally + cryptographically.

**Files:**
- `internal/core/verify/verify.go` — `Verify(ctx, appPath, opts, execer) (*VerifyReport, error)`
- `internal/core/verify/verify_test.go`

**Checks:**
1. Plist consistency: `CFBundleExecutable` resolves to a file under `Contents/MacOS/`.
2. `codesign --verify --deep --strict --verbose=2` on main bundle.
3. Optional: `spctl --assess --type execute` (reported, not a pass/fail for ad-hoc).
4. Optional: launch test via `open -a`, kill after N seconds (off by default).

**Acceptance:** structured result separates "verified" (codesign) from "assessed" (spctl) so the ad-hoc caveat is visible.

**Commit:** `feat(core): add verification pipeline`

## Task 10 — Doctor: failure categorization

**Objective:** Evidence → human-readable diagnosis with severity + suggested fix.

**Files:**
- `internal/core/doctor/doctor.go` — `Diagnose(report) []Finding`
- `internal/core/doctor/rules.go` — rule catalog
- `internal/core/doctor/doctor_test.go`
- `docs/failure-modes.md`

**Findings:**
```go
type Finding struct {
    Code     string   // stable id, e.g. "sparkle_present"
    Title    string
    Severity string   // info | warn | error
    Category string   // updater | sandbox | signature | helper | executable
    Evidence []string // paths / commands that triggered this
    Fix      string   // free-text suggestion
}
```

**Initial rules:**
- `sparkle_present` — Sparkle framework present → auto-update broken
- `electron_helper` — helper bundle IDs rewritten
- `sandbox_entitled` — `com.apple.security.app-sandbox` true → container isolation complications
- `hardened_runtime` — strict runtime may reject ad-hoc
- `missing_executable` — `CFBundleExecutable` doesn't resolve (fatal)
- `login_item_present` — `LSUIElement` / SMLoginItem patterns need manual re-registration
- `team_id_in_entitlements` — team ID references after rewrite

**Commit:** `feat(core): add doctor rule engine`

## Task 11 — TUI screens

**Objective:** Wire Bubble Tea screens: app list → form → progress → result.

**Files:**
- `internal/tui/app.go` — root model, screen enum, transitions
- `internal/tui/applist/` — scan `/Applications` + `~/Applications`, list.Model from bubbles
- `internal/tui/form/` — textinput × {name, bundle id, target (optional), display name (optional)}
- `internal/tui/progress/` — consume stage event channel, render `[n/7] …` lines + spinner
- `internal/tui/result/` — success/warning/failure summary, "open in Finder" / "launch" / "copy JSON" shortcuts

**Event bridge:** Core pipeline emits `StageEvent` on a channel; TUI uses `tea.Cmd` that reads the channel and produces `tea.Msg`. Standard Bubble Tea pattern.

**Acceptance:** full interactive flow on a test `.app` — pick → configure → progress → result — without crashing.

**Commit:** `feat(tui): wire applist/form/progress/result screens`

## Task 12 — CLI wire-up + JSON output

**Objective:** Connect Cobra commands to core pipelines; add `--json`, `--dry-run`, `--verbose`.

**Files:**
- `internal/cli/cli.go` — pull in core pipelines
- `internal/cli/render.go` — human text vs JSON rendering
- `internal/cli/*_test.go` — exit code + output shape tests

**Acceptance:** each of `inspect | clone | verify | doctor` works with and without `--json`; exit codes match documented values.

**Commit:** `feat(cli): wire commands with json and dry-run`

## Task 13 — Real-app validation + support matrix

**Objective:** Run the whole thing against real apps; record results.

**Files:**
- `docs/support-matrix.md` — one row per app: name, type, result, notes, known issues

**Initial targets:** cmux (known good), one Electron app, one Sparkle-updated app, one helper-heavy app, one sandboxed app (expected failure, useful for doctor).

**Acceptance:** all five covered in the matrix with reproducible commands.

**Commit:** `docs: add support matrix from real-app validation`

---

## Working Protocol

- One task per branch is overkill for this size; commit per task on `main` is fine.
- TDD where tests have clear shape (plist, discovery, planning, entitlement filter); skip TDD where it's just orchestration (main loop, screen wiring).
- After each task: `go vet ./... && go test ./... && go build ./...` must all pass before commit.
- Don't commit without running the binary when the task touches the CLI or TUI.
- Keep `internal/core/*` import-clean of UI — `go list -deps` check each round.
