# Security review: ProbeProgram (cli-flag-discovery)

Reviewer: read-only. Skills `security-review`/`golang-security` not used; manual review plus throwaway Go tests in the session scratchpad (copies of `config/clihelp`, `probeguard.go`; nothing added to the repo, no server started, :8543 untouched).

Counts: CRITICAL 0, HIGH 0, MEDIUM 1, LOW 8, INFO 6.

## Bottom line
An attacker who can POST to :8543 (loopback Host, no Origin) can run any regular, executable, non-world-writable file the server user can reach, with `--help`, an allowlisted env and an empty cwd. `confirm_execute` is a request field, so it is UX, not a control. This adds nothing beyond what the same caller already has on the unauthenticated listener (ADR-001 asserts create_session is available there; INFERRED, not tested by me). No shell, argument, or env-secret injection path found. DNS rebinding/CSRF guard held against every variant tried.

## MEDIUM

### M1. Confirm-before-execute gate is client-asserted (ADR-001 overstates it)
- `config/clihelp/execute.go:76-88` (`mayRun`), `server/services/defaults_service.go:824`.
- Scenario: any caller sends `{"command":"/tmp/x.sh","confirm_execute":true}`; the script runs and its confirmation is remembered for the process lifetime. The gate protects only against the UI running a script on blur.
- Fix: keep as UX, but do not present it as a security boundary; add the file-trust checks in L1 as the real boundary, or require a server-issued, single-use confirmation token bound to (path, mtime, size).
- Verified: VERIFIED (Prober test: no flag -> NEEDS_CONFIRM, script not run; `ConfirmExecute:true` -> script ran, marker file created).

## LOW

### L1. No ownership or parent-directory checks on the target
- `config/clihelp/prober.go:178` (`usableExecutable`), `:185` (`lookInDirs`).
- Only "regular, x-bit, not world-writable (file)". Files owned by another uid, group-writable files, and files in world-writable dirs (e.g. `/tmp`, sticky) pass. Another local user can drop a 0755 script in /tmp and have the server user run it (subject to the "gains nothing extra" caveat above).
- Fix: require owner == server uid or root, no group/other write bit, and every ancestor dir not group/other-writable (or root/uid-owned sticky-free).
- Verified: INFERRED from reading (a 0777 dir was not separately tested; result came from cache).

### L2. Grandchild using `setsid` escapes the group kill and outlives the probe
- `config/clihelp/runner.go:131-132`, `runner_unix.go:28`.
- A helper doing `setsid sleep 30` survived the run (pgrep showed it). Temp dir was removed anyway. Also `Pdeathsig` is lost because `newProbeCmd` replaces `SysProcAttr`, so children are not killed if the server is SIGKILLed.
- Fix: set `Pdeathsig: SIGKILL` in the replacement SysProcAttr; accept setsid escapes or use cgroups/PR_SET_CHILD_SUBREAPER.
- Verified: VERIFIED (test script spawned `setsid sleep 30`, still running after Run returned).

### L3. Two-slot semaphore is trivially saturable; no per-caller rate limit
- `config/clihelp/prober.go:17`, `execute.go:104-109`.
- A caller that varies file mtime (or uses distinct scripts) defeats the cache and keeps both slots busy for 3s each; legit probes get BUSY.
- Fix: per-remote-addr token bucket in the handler; cache negative/timeout results by path only for a short TTL.
- Verified: INFERRED.

### L4. Feature-flag kill switch reloads config from disk on every request
- `server/services/feature_flag_service.go:42` (`config.LoadConfig()` per call).
- Cheap amplification on an unauthenticated endpoint (it runs even for requests the guard admits). Flag default is ON (`featureFlagDefault`).
- Fix: cache the flag with a short TTL or read from the in-memory config.
- Verified: INFERRED.

### L5. Audit log does not identify the caller; ADR-001 says it does
- `config/clihelp/prober.go:198-214`. Logged: resolved_path, status, flags, duration, cache_hit, truncated, confirmed, resolve_only. No remote address and no first token (ADR-001 "Audit" says both). Unresolvable commands log an empty `resolved_path`, so attempts to probe missing/unsafe paths are invisible.
- Log-injection: not exploitable; slog escapes values (JSON handler) and `Resolve` rejects `\n`. No env values or args are logged (VERIFIED by reading; test output showed the exact line).
- Fix: pass `r.RemoteAddr` and the first token (length-capped) into `logProbe`.
- Verified: VERIFIED by reading and by test log output.

### L6. Guard admits any loopback Origin on any port
- `server/middleware/probeguard.go:92-98`.
- A page served from `http://localhost:<any>` (another local web app, or an XSS'd one) passes the guard. Browser CORS preflight still blocks it unless the origin is in the configured list, so practical risk is low.
- Fix: compare scheme+host+port against the server's own origin(s), not just hostname.
- Verified: VERIFIED (`Origin: http://localhost:9999` with Host localhost:8543 -> 200).

### L7. Guard rejection log takes attacker-controlled Host/Origin values
- `server/middleware/probeguard.go:34-35`. Values go through slog (escaped) but are uncapped (up to header limit) and a rebinding page can flood the log.
- Fix: truncate to e.g. 128 chars; rate-limit the warn.
- Verified: INFERRED.

### L8. 0xCAFEBABE treated as native
- `config/clihelp/execute.go:14`. It is also the Java class magic; on hosts with `binfmt_misc` for Java the "native" file would be run without confirmation. Cosmetic given M1, but it undermines the stated gate.
- Fix: drop fat-binary magic unless on macOS, or validate the fat header (nfat_arch small).
- Verified: VERIFIED that `isNative` returns true; binfmt behaviour INFERRED.

## INFO

- I1. Env allowlist works: with `SECRET_TOKEN` and `ANTHROPIC_API_KEY` set in the parent, the child saw only PATH, HOME, TERM, NO_COLOR, CI, COLUMNS, LANG, LC_ALL, PAGER, MANPAGER (+ shell-added PWD, SHLVL, _). VERIFIED. Note: the PATH value is the full login PATH (may reveal directory names). The private HOME is not a sandbox: the child runs as the server uid and can read the real home by absolute path.
- I2. Temp dir: `MkdirTemp` (random name) with 0700 cwd/home; child `ls -ld` showed `drwx------`; no `clihelp-probe-*` dirs left afterwards. VERIFIED.
- I3. Output cap and kill: `yes` was killed at the 256 KiB cap in ~3 ms (Truncated=true); `sleep 20` was killed at 3.0 s (TimedOut=true). VERIFIED. Parser is bounded (`maxFlags`=500, line cap 2 KiB); adversarial 260 KB inputs parsed in <0.5 ms. VERIFIED.
- I4. Resolve: first token only, no shell. `/bin/ls;id`, `$(id)` and backticks stay literal file names (no such file -> not found). `~user/x`, `./x`, `..`, `a/../b`, newline tokens, `-h` all rejected. Quirk: `~/../../tmp/evil` expands to `/tmp/evil` (escapes home) but absolute paths are allowed anyway. VERIFIED.
- I5. Guard matrix, all as intended (VERIFIED with a copy of `probeguard.go`): 403 for `localhost.evil.com`, `evil.com`, `127.0.0.1.evil.com`, `localhost@evil.com`, decimal `2130706433`, `127.1`, `0.0.0.0`, empty Host, `Origin: null`, `Origin: http://127.0.0.1@evil.com`; 200 for `[::1]:8543`, `[::1]`, `LOCALHOST`, `localhost.`, `[::ffff:127.0.0.1]`; GET -> 405. Path variants (`//api/...`, `/./`, `/../`, `%50`, trailing slash) do not bypass: they either match the guard (403) or 307/404 and re-enter the guard. Tested against a stand-in mux + exact-path handler, not the real Connect handler (INFERRED for the real one).
- I6. Login-PATH derivation: constant script, server's own `$SHELL`, 2 s timeout, 64 KiB cap, process-group kill, sentinel parse drops relative entries (`config/clihelp/loginpath.go`, `loginpath_unix.go`). An attacker needs to control the user's rc files or `$SHELL` to influence it, which already means code execution. Remote listener chain has no guard by design (`server/server.go:1662`); `main.go:1542` always passes `middleware.Auth`, but `remoteChain` would silently run unauthenticated if a nil `authMW` were ever passed.
- Also noted: TOCTOU between `readHead` (`prober.go:216`) and exec, and confirmation keyed by (path, mtime, size), only matter for a file swappable by someone other than its owner; they are moot given M1 and L1.
