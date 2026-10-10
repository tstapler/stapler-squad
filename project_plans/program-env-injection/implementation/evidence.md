# Evidence: program-env-injection, Epic 1.1

Header
- Implementation base HEAD: 8d0c20afd43df878145e840c2fa1bbe3bb0db390 (test commits: 98ef1b36ba9fcdd7fe5080db7ce9669392da6256, 03261e54746ede35baf078c1b3deb6cf44e29235)
- TMUX_BIN: /home/linuxbrew/.linuxbrew/bin/tmux; `tmux -V`: tmux 3.6a (3.6a only; CI pins 3.4)
- STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30; go version go1.26.6 linux/amd64
- Generated code: gen/proto/go, session/ent/*.go, server/web/dist already present in the worktree (not regenerated, not committed)
- Custom linter: built with `go -C tools/lint build -o <scratch>/linter ./cmd/linter` (Makefile:814-821 recipe, scratch output). Gate script: plan Task 1.1.0b verbatim, self-check on /dev/null printed `GATE FAIL: want 1 "--- PASS: X" line(s), got 0`.
- Test file hashes: `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` file (session_service_create_test.go) = 04af2d1db55712f743d41e03e24958f1636fd0a5; session_service_create_settings_env_test.go = 8febc72370ebc454f3fc8a34862133db21c682ce.
- Baseline before conversion: linter on ./server/services reported 5 findings (norawexec 729, 738, 741; notimesleeptest 718, 740); after: `linter ./server/services ./session ./session/tmux` exit 0.
- Overlays are scratch-only env-only mutations of session/instance_tmux.go; the tree was clean after every run (git status --short empty).
- Coordinator-owned file: this section was written by the implementation agent for Epic 1.1 and is to be reviewed/merged by the coordinator.

| Section | Purpose (AC) | Command | tmux -V | Test-file hash | Result | Gate verdict |
|---|---|---|---|---|---|---|
| E1 | HEAD baseline, converted test (AC1, AC2) | `go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v` | 3.6a | 04af2d1db55712f743d41e03e24958f1636fd0a5 | PASS | GATE OK: 1 "--- PASS" |
| E2 | Repeat-run, -count=5 (AC1, AC2) | same, -count=5 | 3.6a | 04af2d1db55712f743d41e03e24958f1636fd0a5 | PASS x5 | GATE OK: 5 "--- PASS" |
| E3 | PreFixOverlay equivalence (AC3, AC5) | git show / git grep / diff -u | n/a | n/a | see E3 | n/a (static) |
| E4 | Red under overlay (AC3) | same with `-overlay overlay-prefix.json` | 3.6a | 04af2d1db55712f743d41e03e24958f1636fd0a5 | FAIL on intended assertion | GATE OK: 1 "--- FAIL" + grep -F match |
| E5 | Green on HEAD, clean tree (AC3) | as E1 | 3.6a | 04af2d1db55712f743d41e03e24958f1636fd0a5 | PASS | GATE OK: 1 "--- PASS" |
| E6 | #852 unit tests (AC4a) | go test ./session -run '...' -count=1 -v | n/a | n/a | 4 PASS | 4 PASS, 0 SKIP |
| E7 | Upstream precedence quote (AC4b, doc only) | read_website | n/a | n/a | quoted | n/a |
| E8 | --settings end to end, red/green (AC4a) | `go test ... -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$'` +/- overlay-nosettings.json | 3.6a | 8febc72370ebc454f3fc8a34862133db21c682ce | red then green | GATE OK fail-mode and pass-mode |
| E9 | Real claude -p precedence probe (AC4b) | needs a human with credentials | n/a | n/a | NOT RUN | n/a |

AC4b: UNVERIFIED (upstream-documented, not executed); owner acceptance requested/recorded.

### E1 (converted test, HEAD, real tmux)
Command: go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v
```
21:    session_service_create_test.go:749: show-environment:
22-        ANTHROPIC_BASE_URL=http://127.0.0.1:47000
23-        CLAUDECODE=
24-        DISPLAY=:1
25-        -KRB5CCNAME
26-        -MSYSTEM
27-        -SSH_AGENT_PID
28-        -SSH_ASKPASS
29-        SSH_AUTH_SOCK=/home/tstapler/.1password/agent.sock
30-        -SSH_CONNECTION
31-        SSQ_PROGRAM_ENV_PROBE=probe-7f3a91
32-        STAPLER_SESSION_UUID=e283a2cf-ead0-40a7-9285-4c9e26da3395
33-        -WINDOWID
...
31:        SSQ_PROGRAM_ENV_PROBE=probe-7f3a91
35:    session_service_create_test.go:756: pane capture:
36-        echo ENVPROBE_$(printenv SSQ_PROGRAM_ENV_PROBE)_END
37-        [tstapler@onyx program-env-repro_18dd3eeab7761d93]$ echo ENVPROBE_$(printenv SSQ_PROGRAM_ENV_PROBE)_END
38-        ENVPROBE_probe-7f3a91_END
39-        [tstapler@onyx program-env-repro_18dd3eeab7761d93]$ echo ENVPROBE_$(printenv SSQ_PROGRAM_ENV_PROBE)_END
40-        
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (1.60s)
PASS
ok  	github.com/tstapler/stapler-squad/server/services	2.791s
GATE OK: 1 "--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession" line(s), 0 SKIP
```

### E2 (-count=5 -race)
Isolation: each iteration gets its own envtest state dir and temp worktree root; after the run `git worktree list | grep program-env-repro` and `git branch --list 'program-env-repro*'` printed nothing.
```
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (1.23s)
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (0.83s)
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (0.74s)
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (0.72s)
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (0.72s)
ok  	github.com/tstapler/stapler-squad/server/services	5.417s
GATE OK: 5 "--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession" line(s), 0 SKIP
```

### E3 (PreFixOverlay equivalence)
The overlay is an env-only reconstruction on HEAD: at cdfd4e5cf2^ custom program IDs were not resolved to a command at all, so "command resolves, env absent" never existed in history. Only the overlay (E4) isolates the env-only failure.
```
$ git show cdfd4e5cf2^:session/instance_tmux.go | grep -n SetExtraEnv -B2 -A1
577-	}
578-	if i.UUID != "" {
579:		session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})
580-	}
$ git grep -n ResolveProgramConfig cdfd4e5cf2^ -- *.go | grep -v _test
(exit 1)
$ git show cdfd4e5cf2^:session/instance_tmux.go | grep -c "resolveExtraEnvVars\|buildExtraEnv"
0
$ diff -u session/instance_tmux.go <scratch>/instance_tmux.prefix.go
--- session/instance_tmux.go	2026-10-10 10:46:18.560938274 -0700
+++ <scratch>/instance_tmux.prefix.go	2026-10-10 11:38:38.455074802 -0700
@@ -791,7 +791,11 @@
 	} else {
 		session = tmux.NewTmuxSessionWithPrefix(snap.Title, program, tmuxPrefix, opts...)
 	}
-	if extraEnv := i.buildExtraEnv(); len(extraEnv) > 0 {
+	var extraEnv []string
+	if i.UUID != "" {
+		extraEnv = []string{"STAPLER_SESSION_UUID=" + i.UUID}
+	}
+	if len(extraEnv) > 0 {
 		session.SetExtraEnv(extraEnv)
 	}
 	if tb, ok := i.processManager.(*TmuxBackend); ok {
```

### E4 (red under PreFixOverlay)
Command: go test -race -short -overlay <scratch>/overlay-prefix.json ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v  (rc=1)
```
    session_service_create_test.go:749: show-environment:
        CLAUDECODE=
        DISPLAY=:1
        -KRB5CCNAME
        -MSYSTEM
        -SSH_AGENT_PID
        -SSH_ASKPASS
        SSH_AUTH_SOCK=/home/tstapler/.1password/agent.sock
        -SSH_CONNECTION
        STAPLER_SESSION_UUID=fc1bf060-2a2d-4902-8015-a58447a1d274
        -WINDOWID
        XAUTHORITY=/run/user/1000/xauth_aPQnmU
    session_service_create_test.go:750: 
        	Error Trace:	/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-4bbe28f9-09b2-406d-9a72-bdb24df5d509_18d833e2296bcf5a/server/services/session_service_create_test.go:750
33-    session_service_create_test.go:750: 
34:        	Error Trace:	server/services/session_service_create_test.go:750
35-        	Error:      	"CLAUDECODE=\nDISPLAY=:1\n-KRB5CCNAME\n-MSYSTEM\n-SSH_AGENT_PID\n-SSH_ASKPASS\nSSH_AUTH_SOCK=/home/tstapler/.1password/agent.sock\n-SSH_CONNECTION\nSTAPLER_SESSION_UUID=fc1bf060-2a2d-4902-8015-a58447a1d274\n-WINDOWID\nXAUTHORITY=/run/user/1000/xauth_aPQnmU\n" does not contain "ANTHROPIC_BASE_URL=http://127.0.0.1:47000"
36-        	Test:       	TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession
37-        	Messages:   	tmux show-environment must carry the program's env
38-    session_service_create_test.go:751: 
39:        	Error Trace:	server/services/session_service_create_test.go:751
40-        	Error:      	"CLAUDECODE=\nDISPLAY=:1\n-KRB5CCNAME\n-MSYSTEM\n-SSH_AGENT_PID\n-SSH_ASKPASS\nSSH_AUTH_SOCK=/home/tstapler/.1password/agent.sock\n-SSH_CONNECTION\nSTAPLER_SESSION_UUID=fc1bf060-2a2d-4902-8015-a58447a1d274\n-WINDOWID\nXAUTHORITY=/run/user/1000/xauth_aPQnmU\n" does not contain "SSQ_PROGRAM_ENV_PROBE=probe-7f3a91"
41-        	Test:       	TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession
42-        	Messages:   	tmux show-environment must carry the probe key
43-    eventually.go:58: wait: still waiting after 9.909s (up to 9.448s remaining, load factor 1.3x) -- if this repeats, the machine is likely contended, not the condition stuck
44-    session_service_create_test.go:757: timeout waiting for condition after 19.515976123s (base 15s, scaled to 19.35625s for load): printenv inside the pane must show the program's env
45-    session_service_create_test.go:756: pane capture:
46-        [tstapler@onyx program-env-repro_18dd3f1a392d7c75]$ echo ENVPROBE_$(printenv SSQ_PROGRAM_ENV_PROBE)_END
--- FAIL: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (20.83s)
FAIL
FAIL	github.com/tstapler/stapler-squad/server/services	21.079s
FAIL
GATE OK: 1 "--- FAIL: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession" line(s), 0 SKIP
        	Messages:   	tmux show-environment must carry the program's env
```
Resolver unit test stays green under the same overlay (PIT-5): go test -overlay <scratch>/overlay-prefix.json ./session -run '^TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars$' -count=1 -v
```
--- PASS: TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars (0.00s)
ok  	github.com/tstapler/stapler-squad/session	0.150s
```
E4b (parent-commit run at cdfd4e5cf2^): coordinator-reported, no verbatim record, unconverted test, fails for two reasons (ID unresolved + env absent). NOT RUN here; not counted toward AC3.

### E5 (green on HEAD, clean tree)
```
39:        ENVPROBE_probe-7f3a91_END
--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession (2.04s)
ok  	github.com/tstapler/stapler-squad/server/services	3.252s
GATE OK: 1 "--- PASS: TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession" line(s), 0 SKIP
$ git status --short   (empty, prior to evidence.md being added)
```

### E6 (#852 unit tests)
Command: go test ./session -run 'TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars|TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars' -count=1 -v
```
--- PASS: TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars (0.00s)
--- PASS: TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars (0.00s)
--- PASS: TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars (0.00s)
--- PASS: TestBuildClaudeCommand_IncludesSettingsEnvOverride (0.00s)
ok  	github.com/tstapler/stapler-squad/session	0.180s
```

### E7 (upstream precedence, documentation only; does NOT verify AC4b)
Source: https://code.claude.com/docs/en/settings ("How scopes interact"), fetched 2026-10-10 with mcp__stapler-mcp__read_website.
```
When the same setting appears in multiple scopes, Claude Code applies them in priority order:

1.  **Managed** (highest): can’t be overridden by any other scope, apart from the exceptions under [Settings precedence](#settings-precedence)
2.  **Command line arguments**: temporary session overrides
3.  **Local**: overrides project and user settings
4.  **Project**: overrides user settings
5.  **User** (lowest): applies when nothing else specifies the setting
```
AC4b: UNVERIFIED locally (documented, not executed). FakeClaude never reads settings.json. Note the page names "Command line arguments" without enumerating `--settings` in that list; the in-repo comment (session/instance_tmux.go claudeSettingsEnvOverrideArgs) is the stated link. Managed settings still outrank it.

### E8 (--settings end to end, FakeClaude through real tmux and a real shell)
tmux -V: tmux 3.6a; `tmux show-options -gv default-shell`: /bin/zsh; $SHELL: /bin/zsh.
Green command: go test -race -short ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v
```
21:    session_service_create_settings_env_test.go:100: recorded argv (one element per line):
22-        --settings
23-        {"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:47000","SSQ_HOSTILE":"it's $(touch /tmp/TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunched2882724139/003/pwned) `touch /tmp/TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunched2882724139/003/pwned` \"q\" a=b"}}
24-2026/10/10 11:42:30 INFO [session pipeline] async start complete session=settings-env-repro
--- PASS: TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess (0.70s)
ok  	github.com/tstapler/stapler-squad/server/services	1.832s
GATE OK: 1 "--- PASS: TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess" line(s), 0 SKIP
```
Red command: go test -race -short -overlay <scratch>/overlay-nosettings.json ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v (claudeSettingsEnvOverrideArgs returns "", "" first; rc=1)
```
23-        
24-    session_service_create_settings_env_test.go:110: 
25:        	Error Trace:	server/services/session_service_create_settings_env_test.go:110
26-        	Error:      	"-1" is not greater than or equal to "0"
27-        	Test:       	TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess
28-        	Messages:   	argv must carry the --settings override
29-2026/10/10 11:42:25 INFO [ApprovalPersistence] persisted approvals count=0 path=/tmp/TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunched3687536320/001/pending_approvals.json
30-2026/10/10 11:42:25 INFO killing orphaned tmux attach process session=staplersquad_settings-env-repro pid=131336
31-2026/10/10 11:42:25 INFO attach-session process exited session=staplersquad_settings-env-repro exitErr="signal: killed"
--- FAIL: TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess (1.32s)
FAIL
FAIL	github.com/tstapler/stapler-squad/server/services	1.534s
FAIL
GATE OK: 1 "--- FAIL: TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess" line(s), 0 SKIP
```
Hostile-value integrity: recorded argv equals the registered env exactly (assert.Equal on the unmarshalled JSON) and the pwned file is absent (assert.True os.IsNotExist).

### E9 (real claude -p precedence probe, AC4b)
NOT RUN. Needs a human with a real claude login. Probe: scratch CLAUDE_CONFIG_DIR with settings.json {"env":{"SSQ_PRECEDENCE_PROBE":"global"}}; run claude -p with --settings '{"env":{"SSQ_PRECEDENCE_PROBE":"cli"}}' and print $SSQ_PRECEDENCE_PROBE via its Bash tool: "cli" proves AC4b, "global" disproves it.

### Lint
```
$ <scratch>/linter ./server/services ./session ./session/tmux   -> exit 0, no output
$ go vet ./server/services                                      -> exit 0
$ gofmt -l server/services                                      -> no output
$ golangci-lint run ./server/services/...                       -> 0 issues.
```

## sdd:6-verify (coordinator, 2026-10-10, tmux 3.6a, HEAD after test edits)

- Layer 1 (Go idioms) and Layer 2 (architecture, refactor candidates): 0 MUST FIX, 0 BLOCKER. Applied: full-value hostile assert, `assert.NoFileExists`, naming comment. Not applied (deliberate): shared helper extraction (two callers only), fail-instead-of-skip when tmux is absent in CI (covered by the RealTmuxGate in this plan; CI pins tmux).
- Layer 3 (coordinator-run, `-race -short`, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`):
  - `go test ./server/services ./session ./session/tmux -count=1`: all three `ok` (162s / 97s / 45s); `go build ./...` rc=0; `go vet` clean; `gofmt -l` empty.
  - Both real-tmux tests `--- PASS` with no SKIP; custom linter `./server/services ./session ./session/tmux` rc=0.
- AC4b remains UNVERIFIED (E9 not run). E4b not re-run.
