---
title: 'macOS Code Signing for TCC Permission Persistence'
description: 'stapler-squad signs its binary with a self-signed certificate (StaplerSquadDev) on every make install-service.'
---

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

### Troubleshooting: "MAC verification failed during PKCS12 import (wrong password?)"

OpenSSL 3.x's PKCS12 export produces a MAC that macOS's `security import` cannot verify when the export password is empty — a known OpenSSL 3.x/macOS incompatibility. `setup-codesign.sh` works around this by generating a random throwaway password with `openssl rand -base64 24` and using it for both the `pkcs12 -export` and `security import` steps (the password only protects the `.p12` file in transit between those two commands and is discarded immediately after). If you still see this error, confirm `$OPENSSL_BIN` (or `openssl` on `PATH`) is real OpenSSL and not a stale/patched build — `openssl version` should print `OpenSSL 3.x`, not `LibreSSL`.

## Verifying the setup

After `make install-service`, run:

```bash
make verify-codesign
```

Focus Window uses Launch Services (`lsappinfo` + `open`), not Apple Events, so macOS shows no per-app "would like to access…" dialog and no Automation entitlement is needed. Signing still keeps other TCC grants (e.g. file access) stable across rebuilds; without `make setup-codesign` those may re-prompt after each rebuild.

Pass criteria:
- `Authority=StaplerSquadDev` in the Code Signature section
- `Identifier=com.stapler-squad` in the Code Signature section
- DR section contains `anchor H"<cert-sha1>"` or `anchor trusted` (cert-anchored, not `cdhash`)
- `CFBundleIdentifier => "com.stapler-squad"` in the Embedded Info.plist section

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
4. Re-run your workflow; Focus Window no longer needs an Apple Events grant

## How it works

The `make install-service` flow on macOS:

1. `go build` with `CGO_LDFLAGS="-sectcreate __TEXT __info_plist macos/Info.plist"` embeds the plist into the binary's `__TEXT/__info_plist` Mach-O section
2. `otool` assertion verifies the plist was actually embedded (catches silent `CGO_ENABLED=0` failures)
3. `codesign --sign "StaplerSquadDev" --entitlements entitlements.plist` signs the binary
4. `install-service.sh` stops, installs, and restarts the LaunchAgent

The `CFBundleIdentifier = com.stapler-squad` in the embedded plist causes TCC to track the app by bundle ID (`client_type=0`) rather than path+hash. The cert-anchored DR means the TCC row's `csreq` field matches every rebuild as long as the same cert is used.

**Why `Info.plist` lives in `macos/` and not the repo root:** if the source `Info.plist` file sits in the same directory as the built binary, `codesign` auto-detects a bundle layout and seals the *entire directory* as bundle resources (every file, recursively — this repo has 80k+ tracked/untracked files). `codesign --verify` then fails with `a sealed resource is missing or invalid` the moment anything in the repo changes (a git worktree's `.git/index`, an untracked `.claude/settings.local.json`, etc.), which silently aborts `install-service.sh`'s pre-flight signature check before it ever restarts the LaunchAgent. Keeping `Info.plist` out of the binary's directory keeps `codesign` treating it as a plain Mach-O executable (`Sealed Resources=none`).

## Bazel-built binary (owner device check; not done yet)

Signing stays outside Bazel (ADR-005, ADR-006): Bazel produces the unsigned binary and the existing flow signs a copy.
Nothing in this section has been run on a Mac. The Bazel side (`--config=macos`, the `-sectcreate` Info.plist link
in `tools/bzl/otel_weave_hermetic.sh`, `//macos:infoplist_test`, `.github/workflows/bazel-macos.yml`) was written
without one and is unverified, including whether the woven (otelc) darwin build links at all; cross-compiling darwin
from Linux is not supported (cgo is off there, so no Info.plist).

### Designated requirement of the signed copy

`BUILD_SYSTEM=bazel make install-service` runs `tools/bzl/verifybin -codesign`, which aborts before the old service is
stopped unless `codesign -d -r-` on the signed copy prints exactly the requirement recorded here. Until the owner
records it the check fails closed (it names the missing marker). Record it once, from the make-built signed binary:

```bash
make install-service            # BUILD_SYSTEM=make, once, so the binary is signed by StaplerSquadDev
codesign -d -r- ./stapler-squad # copy the "designated =>" line into the block below
```

<!-- designated-requirement:begin -->
Not recorded yet (owner: replace this line with the `designated =>` line).
<!-- designated-requirement:end -->

### Checklist (owner, per architecture: arm64 and amd64)

1. Build with Bazel and copy to a stable path (not `bazel-bin`):
   `bazel build --config=release //:stapler-squad && cp bazel-bin/stapler-squad /path/stable/stapler-squad-bazel`
   (add `--config=darwin_amd64` for Intel).
2. Sign the copy: `make _codesign-binary BIN=/path/stable/stapler-squad-bazel`, then
   `codesign --verify --strict /path/stable/stapler-squad-bazel` must exit 0.
3. Compare requirements, and read the TCC guidance it prints:
   `bazel run //tools/bzl/codesigndiff -- -make-bin ./stapler-squad -bazel-bin /path/stable/stapler-squad-bazel`
   (exit 0 and `IDENTICAL`).
   Also run `scripts/verify-bazel-codesign.sh /path/stable/stapler-squad-bazel ./stapler-squad`: it checks the
   signature, the `StaplerSquadDev` authority and a certificate-anchored (not `cdhash`) requirement, and that the
   requirement equals the make-built binary's. It exits 2 with a message anywhere but macOS.
4. With the make-built binary's TCC grants in place, replace it with the Bazel-built signed binary and relaunch. PASS
   only if macOS shows no new permission prompt. Do not run `make tcc-reset`.
5. Record the result below and in the PR.

| Architecture | Date | Result (no new prompt?) | Notes |
|---|---|---|---|
| arm64 | not run | | |
| amd64 | not run | | |

### Owner sign-off (OWNER-RUN, unverified by agents)

Agents cannot run any of this: it needs a Mac, the login keychain with `StaplerSquadDev`, and live TCC state. The
script's non-macOS exit and its pass/fail logic against a stub `codesign` are tested in
`scripts/verify-bazel-codesign_test.sh`; the real `codesign` output format is not.

- [ ] arm64: step 1-3 of the checklist, `scripts/verify-bazel-codesign.sh` exits 0
- [ ] arm64: step 4, no new permission prompt after swapping in the signed Bazel binary
- [ ] amd64 (`--config=darwin_amd64`): the same two items
- [ ] The `designated =>` block above is recorded, so `verifybin -codesign` no longer fails closed
- [ ] Results entered in the table above and in the PR
