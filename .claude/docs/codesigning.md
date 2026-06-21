# macOS Code Signing for TCC Permission Persistence

## What this does

stapler-squad signs its binary with a self-signed certificate (`StaplerSquadDev`) on every `make install-service`. The certificate anchors the Designated Requirement (DR), so macOS TCC grants (Full Disk Access, Apple Events) survive rebuilds without re-prompting.

Without a stable cert-anchored DR, every rebuild changes the binary's cdhash and macOS treats it as a new app — requiring a fresh round of permission dialogs.

## One-time setup

Run once per developer machine:

```bash
# Install OpenSSL if needed (macOS ships LibreSSL, which won't work)
brew install openssl

# Create and trust the self-signed cert
OPENSSL_BIN=$(brew --prefix openssl)/bin/openssl make setup-codesign
```

If `openssl version` shows `OpenSSL` (not `LibreSSL`), you can omit `OPENSSL_BIN`:

```bash
make setup-codesign
```

The script is idempotent — running it a second time prints "StaplerSquadDev cert already present." and exits without creating a duplicate.

## Verifying the setup

After `make install-service`, run:

```bash
make verify-codesign
```

Pass criteria:
- `Authority=StaplerSquadDev` in the Code Signature section
- `Identifier=com.stapler-squad` in the Code Signature section
- DR section contains `anchor H"<cert-sha1>"` or `anchor trusted` (cert-anchored, not `cdhash`)
- `CFBundleIdentifier => "com.stapler-squad"` in the Embedded Info.plist section
- `com.apple.security.automation.apple-events` in the Entitlements section

## Exporting the cert for backup

If you need to move the cert to another machine or back it up:

1. Open **Keychain Access** (`/Applications/Utilities/Keychain Access.app`)
2. Select the **login** keychain
3. Find **StaplerSquadDev** in the My Certificates category
4. Right-click → **Export "StaplerSquadDev"**
5. Save as `.p12`, set a password when prompted

## Re-importing on a new machine

```bash
security import StaplerSquadDev.p12 \
    -k ~/Library/Keychains/login.keychain-db \
    -T /usr/bin/codesign
```

Then run `make setup-codesign` (it will detect the cert exists and skip creation, but the set-key-partition-list step ensures codesign can access the key without prompting).

Actually for a clean import, just run the full setup again — the script checks for the cert and exits early if found. You may need to delete the old cert entry first in Keychain Access if re-importing on the same machine.

## Diagnosing TCC issues

### Check current grants

```bash
# List Apple Events grants for stapler-squad
sqlite3 ~/Library/Application\ Support/com.apple.TCC/TCC.db \
    "SELECT client, auth_value, auth_reason FROM access WHERE client = 'com.stapler-squad';"
```

Note: reading TCC.db directly may require Full Disk Access for your terminal.

### Reset grants (development only)

```bash
make tcc-reset
```

This resets all TCC grants for `com.stapler-squad`. Use during development to reproduce the first-launch permission prompt experience. Requires `sudo`.

### Re-grant after reset

1. Run `make install-service`
2. Open System Settings → Privacy & Security → Full Disk Access
3. Enable the toggle for `stapler-squad`
4. Trigger FocusWindow in the UI to re-approve Apple Events for each target app

## How it works

The `make install-service` flow on macOS:

1. `go build` with `CGO_LDFLAGS="-sectcreate __TEXT __info_plist Info.plist"` embeds the plist into the binary's `__TEXT/__info_plist` Mach-O section
2. `otool` assertion verifies the plist was actually embedded (catches silent `CGO_ENABLED=0` failures)
3. `codesign --sign "StaplerSquadDev" --entitlements entitlements.plist` signs the binary
4. `install-service.sh` stops, installs, and restarts the LaunchAgent

The `CFBundleIdentifier = com.stapler-squad` in the embedded plist causes TCC to track the app by bundle ID (`client_type=0`) rather than path+hash. The cert-anchored DR means the TCC row's `csreq` field matches every rebuild as long as the same cert is used.
