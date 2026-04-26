# Release Signing and Notarization

doppel can be built and released without Apple credentials, but ordinary-user
distribution should use Developer ID signing and Apple notarization. Without
that, macOS Gatekeeper may quarantine the downloaded binary and users may need
to run the `xattr` workaround documented in the README.

## Current State

- GoReleaser builds darwin `amd64` and `arm64` tarballs.
- Release artifacts are currently unsigned.
- Homebrew install works, but first launch may be blocked by Gatekeeper.
- `goreleaser check` validates the config, with only the known `brews`
  deprecation notice.

## Target State

- Build macOS artifacts in CI.
- Sign each darwin binary with a Developer ID Application certificate.
- Package signed artifacts into notarizable ZIP archives.
- Submit each ZIP to Apple notarization with `xcrun notarytool submit --wait`.
- Ship only artifacts that pass notarization.
- Keep the Homebrew caveat until signed/notarized releases are verified on a
  clean macOS machine.

## Apple Requirements

Apple's Developer ID path is required for software distributed outside the Mac
App Store. Notarization uses Apple's notary service to scan the signed software
and produce a ticket that Gatekeeper can trust.

Required local/CI capabilities:

- Apple Developer Program membership.
- Developer ID Application certificate and private key.
- Xcode or command-line tools with `xcrun notarytool`.
- Notary authentication, preferably an App Store Connect API key or a saved
  notarytool keychain profile.

## GitHub Secrets

Use repository or environment secrets. Do not commit certificates or API keys.

| Secret | Purpose |
|---|---|
| `MACOS_CERTIFICATE_P12` | Base64-encoded Developer ID Application `.p12`. |
| `MACOS_CERTIFICATE_PASSWORD` | Password for the `.p12`. |
| `MACOS_CODESIGN_IDENTITY` | Identity, for example `Developer ID Application: Name (TEAMID)`. |
| `MACOS_NOTARY_KEY_ID` | App Store Connect API key ID. |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect issuer ID. |
| `MACOS_NOTARY_KEY` | Base64-encoded `.p8` private key. |

## Manual Release Check

Run this on a macOS machine with the certificate installed:

```bash
make build
MACOS_CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)" \
MACOS_NOTARY_KEY_PATH=/path/to/AuthKey_XXXX.p8 \
MACOS_NOTARY_KEY_ID=XXXX \
MACOS_NOTARY_ISSUER_ID=YYYY \
scripts/sign-notarize-binary.sh ./doppel ./doppel-notary.zip
```

If you store notarytool credentials in the keychain, use
`MACOS_NOTARY_PROFILE` instead of the API-key variables:

```bash
MACOS_CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)" \
MACOS_NOTARY_PROFILE=doppel-notary \
scripts/sign-notarize-binary.sh ./doppel ./doppel-notary.zip
```

For GitHub Releases and Homebrew, this manual path should be replaced by a
repeatable CI job once Apple credentials are available.

## GoReleaser Note

GoReleaser has native macOS notarization support, but that feature is
GoReleaser Pro. The current open-source workflow should either:

- use a custom macOS signing/notarization script around GoReleaser artifacts, or
- upgrade to GoReleaser Pro and configure `notarize.macos_native`.

Until one of those is done, keep the README and Homebrew caveat explicit about
unsigned binaries.
