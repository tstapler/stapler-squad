# Playwright Chromium Install Hangs During Extraction — Extract Manually with `ditto`

If `npx playwright install chromium` (or `--with-deps`) hangs indefinitely after the download reaches 100%, don't just kill-and-retry the same command repeatedly — it reproduces the identical hang every time. Download the zip yourself and extract it with the native macOS `ditto` tool instead of Playwright's own Node-based extractor.

**Symptom (VERIFIED, reproduced 4x identically on 2026-08-11):**
```bash
$ ps -o pid,etime,time -p <install-pid>
  PID ELAPSED      TIME
56756   01:26   0:00.40          # high elapsed, near-zero CPU time = stalled, not slow

$ find ~/Library/Caches/ms-playwright/chromium-1194 -type f | wc -l
38                                 # always freezes at exactly 38 files / ~624KB,
                                    # stuck mid-write on Contents/Resources/Assets.car
```
The download log always completes cleanly to 100% of the full zip size — the fault is isolated to extraction, never the download.

**Wrong (retried 4x, failed identically every time):**
```bash
kill -9 <pid>
npx playwright install chromium   # repeats the exact same hang
```

**Right — bypass Playwright's Node extractor entirely:**
```bash
curl -fSL -o /tmp/pw-manual/chromium-mac-arm64.zip \
  "https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/<REV>/chromium-mac-arm64.zip"

mkdir -p ~/Library/Caches/ms-playwright/chromium-<REV>/chrome-mac
ditto -x -k /tmp/pw-manual/chromium-mac-arm64.zip ~/Library/Caches/ms-playwright/chromium-<REV>/chrome-mac

# ditto extracts the zip's own top-level chrome-mac/ folder *inside* the target
# chrome-mac dir, so it nests one level too deep — flatten it:
cd ~/Library/Caches/ms-playwright/chromium-<REV>
mv chrome-mac chrome-mac-tmp && mv chrome-mac-tmp/chrome-mac chrome-mac && rmdir chrome-mac-tmp

# Playwright checks for this marker file to consider the browser "installed":
touch ~/Library/Caches/ms-playwright/chromium-<REV>/INSTALLATION_COMPLETE
```
Find the exact download URL and revision number from the failed install's own log output (it prints `Downloading Chromium ... (playwright build v<REV>) ... from <url>` before hanging on extraction).

`ditto` extracted the identical 325-file bundle in under a second — versus the Node extractor never completing after minutes, across four independent attempts. This isolates the fault to Playwright's own extraction step (Node's `extract-zip`/`yauzl`), not the filesystem, not the download, and not disk I/O in general.

## Why

**Root-cause hypothesis (INFERRED, not confirmed against this exact incident's logs):** this machine runs CrowdStrike Falcon (`com.crowdstrike.falcon.Agent`), a macOS Endpoint Security system extension, observed consuming very high sustained CPU (40–170%+) with continuously accumulating total CPU time throughout every reproduction. Endpoint Security extensions hook file-system syscalls (open/write/exec) synchronously, system-wide. Bulk-extracting a `.app` bundle — hundreds of small files, many Mach-O binaries and `.lproj` resources, written in rapid succession — is exactly the workload pattern that exposes per-file EDR authorization latency or an outright synchronous-callback deadlock in Node's extraction path. `ditto` is a first-party Apple tool and may take a different (or exempted) syscall path through the same Endpoint Security hooks.

This is a hypothesis, not a proven cause — no direct causal test (comparing extraction with Falcon paused/excluded) has been run. **Do not attempt to disable, exclude, kill, or otherwise circumvent the EDR agent without explicit IT authorization** — if this keeps recurring, request an IT-managed scan exclusion for `~/Library/Caches/ms-playwright/` instead.

Playwright's browser cache is shared machine-wide (`~/Library/Caches/ms-playwright/`), not per-worktree or per-project — one successful manual extraction fixes every future session/worktree on this machine until the pinned Playwright version changes revisions.

## How to apply

- Diagnose a genuine stall (not just slow) via `ps -o pid,etime,time -p <pid>`: high `ELAPSED` with near-zero `TIME` means the process is blocked, not busy computing.
- Check the actual worker process, not a wrapper — `npx playwright install` spawns a child (`node .../@playwright/test/cli.js install chromium`); inspect that child PID's CPU time and open file descriptors (`lsof -p <pid>`), not the `npm exec` supervisor's.
- One blind kill-and-retry is reasonable to rule out a transient fluke. A second identical stall at the same file count is the signal to stop retrying and switch to the manual `ditto` extraction above.
- The same technique applies to `chromium_headless_shell-<REV>` and any other Playwright browser artifact that hangs the same way — the download URL pattern and marker-file mechanism are the same across browsers.
