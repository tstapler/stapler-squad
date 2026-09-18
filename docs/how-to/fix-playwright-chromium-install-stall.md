# Playwright Chromium Install Hangs During Extraction — Extract Manually with `ditto`

If `npx playwright install chromium` (or `--with-deps`) hangs indefinitely after the download reaches 100%, don't just kill-and-retry the same command repeatedly — it reproduces the identical hang every time (verified: 4 identical reproductions on 2026-08-11, always stalling at the same file count during extraction, never during download). Download the zip yourself and extract it with the native macOS `ditto` tool instead of Playwright's own Node-based extractor.

**Diagnose a genuine stall** (not just slow):
```bash
ps -o pid,etime,time -p <install-pid>   # high ELAPSED + near-zero TIME = blocked, not busy
```
Inspect the actual worker process, not the `npm exec` wrapper — `npx playwright install` spawns a child (`node .../@playwright/test/cli.js install chromium`); check that child PID.

**Fix — bypass Playwright's Node extractor entirely:**
```bash
curl -fSL -o /tmp/pw-manual/chromium-mac-arm64.zip \
  "https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/<REV>/chromium-mac-arm64.zip"

mkdir -p ~/Library/Caches/ms-playwright/chromium-<REV>/chrome-mac
ditto -x -k /tmp/pw-manual/chromium-mac-arm64.zip ~/Library/Caches/ms-playwright/chromium-<REV>/chrome-mac

# ditto nests the zip's own chrome-mac/ folder one level too deep — flatten it:
cd ~/Library/Caches/ms-playwright/chromium-<REV>
mv chrome-mac chrome-mac-tmp && mv chrome-mac-tmp/chrome-mac chrome-mac && rmdir chrome-mac-tmp

# Playwright checks for this marker file to consider the browser "installed":
touch ~/Library/Caches/ms-playwright/chromium-<REV>/INSTALLATION_COMPLETE
```
Find the exact download URL and revision number from the failed install's own log output (it prints `Downloading Chromium ... (playwright build v<REV>) ... from <url>` before hanging on extraction).

The same technique applies to `chromium_headless_shell-<REV>` and any other Playwright browser artifact that hangs the same way — same URL pattern and marker-file mechanism.

Playwright's browser cache is shared machine-wide (`~/Library/Caches/ms-playwright/`), not per-worktree or per-project — one successful manual extraction fixes every future session/worktree on this machine until the pinned Playwright version changes revisions.

## Why (root-cause hypothesis, INFERRED)

This machine runs CrowdStrike Falcon, a macOS Endpoint Security system extension, observed at very high sustained CPU during every reproduction. Endpoint Security extensions hook filesystem syscalls synchronously, system-wide — bulk-extracting a `.app` bundle (hundreds of small files written in rapid succession) is exactly the workload that exposes per-file EDR authorization latency or a synchronous-callback deadlock in Node's extraction path. `ditto`, a first-party Apple tool, apparently takes a different (or exempted) path through the same hooks. This is a hypothesis, not a proven cause — no direct A/B test with Falcon paused has been run.

**Do not attempt to disable, exclude, kill, or otherwise circumvent the EDR agent without explicit IT authorization** — if this keeps recurring, request an IT-managed scan exclusion for `~/Library/Caches/ms-playwright/` instead.

One blind kill-and-retry is reasonable to rule out a transient fluke. A second identical stall at the same file count is the signal to stop retrying and switch to the manual `ditto` extraction above.
