# Spike S5: credentials and transport (Story 0.2.5, ADR-005, gate G5)

Date: 2026-10-08. go-git `v5.19.2` (the version in `go.mod`), Go 1.26.x, macOS arm64, `/usr/bin/git` 2.50.1.
Source: `S5-src/` (throwaway module `s5spike`, package `credential`, 24 tests, copied from `/tmp/s5`). No product code touched; no real tokens or GitHub auth used (all credentials are dummies, hosts are `127.0.0.1` or `*.invalid`).

## Verdict

**G5: GO on mechanism, with live checks still manual.** go-git v5.19.2 needs **no fork patch** for credentials, custom HTTP client, redirect policy, HTTPS-to-SSH fallback or `insteadOf`; every hook used is public (`transport/http.AuthMethod`, `client.InstallProtocol`, `http.ClientOptions`, `FetchOptions.RemoteURL`, `ssh.AuthMethod`). The plan's Go criterion has three parts; here is what is and is not proven:

| G5 criterion | Status | Evidence |
|---|---|---|
| Client works for keychain | PARTIAL: protocol client round-trips and `git-credential-osxkeychain` execs directly with zero `git` spawns (empty reply for a dummy host); no real keychain item read | T-HELPERS |
| Client works for `gh` | VERIFIED on a dummy GHE host with a throwaway `GH_CONFIG_DIR`; returns the dummy token; zero `git` spawns | T-HELPERS |
| Client works for GHE (live) | NOT RUN (forbidden here; stays manual per validation.md G-4). Host scoping proven with an `httptest`-class server and `TokenSource` per host | T-PROVIDER |
| Redirect test passes | VERIFIED, but see finding F1: stock go-git/net/http **forwards** `Authorization` on a same-hostname different-port redirect; our `CheckRedirect` closes it | T-REDIRECT |

Findings F1-F4 below change the plan text (tests to add, one "zero spawn" claim that is false as written). None needs the fork.

## How to reproduce

```sh
cd project_plans/go-git-fork-full-git-replacement/implementation/spikes/S5-src   # or a copy in /tmp/s5
GOFLAGS=-mod=mod go vet ./...
GOFLAGS=-mod=mod go test ./credential -race -count=1 -v
```
Result recorded 2026-10-08: `ok  s5spike/credential  44.854s` with `-race`, 24 `--- PASS`, 0 FAIL (`/tmp/s5/run.txt`). `TestMain` pins `/usr/bin/git`, because on this machine `git` on PATH is the dotfiles `~/.local/bin/git` ssh-fallback wrapper (a `#!/usr/bin/env sh` script that breaks under a restricted PATH; first cause of a spurious `EOF` in the exec-path test).

Harness: `git http-backend` run as CGI behind `net/http` (`gitHTTP`, optional Basic auth, optional redirect); an in-process `golang.org/x/crypto/ssh` server that execs `git-upload-pack` (`newSSHServer`); an in-process ssh-agent on a unix socket (`startAgent`); a spawn shim (`newShim`) that puts logging `git` and `git-upload-pack` first on PATH so any spawn by the process under test is recorded.

## Results by acceptance criterion

### AC1: credential protocol client, zero `git` spawns

`credential.go`: `Request`, `Helper` (`Get`/`Approve`/`Reject` = `get`/`store`/`erase`), `ResolveHelper`, `Provider` (Go-native `TokenSource`s first, helper as always-on fallback, matches ADR-005 and plan Story 3.1.1), `HostAuth` (an `http.AuthMethod`).

- T-HELPERS `TestRealHelpers_should_ExecDirectlyWithZeroGitSpawns_OnDummyHosts`: VERIFIED.
  - `osxkeychain get` on `s5-dummy.invalid` execs `/Library/Developer/CommandLineTools/usr/libexec/git-core/git-credential-osxkeychain`, exit 0, empty reply.
  - `!gh auth git-credential get` with `GH_CONFIG_DIR` pointing at a temp `hosts.yml` (`ghe.s5.invalid`, `oauth_token: dummy-...`) returns `username=dummyuser` and the dummy token; an unconfigured host returns nothing.
  - The spawn shim logged zero `git` execs for both.
- `TestClient_should_RoundTripStubHelperAndCloneWithZeroGitSpawns`: stub `git-credential-s5stub` (shell script, no git) answers `get` for the test server's `host:port`; go-git `PlainClone` with the resulting `HostAuth` succeeds against a Basic-auth server; helper action log is exactly `get`; shim log empty. VERIFIED.
- `TestClone_should_ReturnAuthRequired_When_NoOrWrongCredential`: no or wrong credential yields `transport.ErrAuthenticationRequired` (go-git's 401 mapping, `plumbing/transport/http/common.go:584`); `IsAuthFailure` also covers `ErrAuthorizationFailed` (403). VERIFIED.
- `TestHelper_should_KillProcessGroup_When_HelperTimesOut`: helper that backgrounds `sleep 60`; with `Setpgid` plus `kill(-pgid)` the timeout returns in under 3 s and the grandchild is dead. VERIFIED (validation.md row `Provider_should_KillProcessGroup_When_HelperTimesOut`).
- `TestProvider_should_ScopeTokenSourcesByHostAndFallBackToHelper`: GHE host gets only its own token (never the `github.com` one); a host with no keychain entry falls to the configured helper; unknown host errors. VERIFIED (rows `CredentialFor_should_ReturnGHEToken_When_HostIsGHE`, `..._ExecConfiguredHelper_When_NoKeychainEntry`).
- `TestHostAuth_should_NotAttachCredential_When_RequestHostDiffers`: `SetAuth` attaches only when `r.URL.Host` equals the host it was built for (defence in depth for redirects); `String()` masks the password. VERIFIED.

**F4 (new):** on macOS the built-in `git-credential-osxkeychain` is not on PATH (`which git-credential-osxkeychain` finds nothing); git finds it through `git --exec-path`. Resolving it by spawning `git --exec-path` would break the zero-spawn claim, so `ResolveHelper` searches `KnownExecDirs` (CLT, Xcode, Homebrew, `/usr/lib/git-core`) instead. Go-native keychain (`github.GetKeychainTokenForHost`, `github/keychain.go:131`, uses the `keyringGet` store) is injected as a `TokenSource`; the spike used a stub closure, so the real injection through `session/gitwiring` is UNVERIFIED.

### AC2: network operation through `InstallProtocol` and a custom client

- `TestInstallProtocol_should_RouteGoGitThroughCustomHTTPClient`: `client.InstallProtocol("http"/"https", githttp.NewClientWithOptions(&http.Client{Transport: rt, CheckRedirect: ...}, ...))`. A counting `RoundTripper` saw 2 requests for a clone (info/refs and upload-pack POST), so the custom client is used. VERIFIED.
- `TestPush_should_Succeed_When_HelperAuthOverCustomClient`: clone, commit, `Push` with the same auth over the custom client; remote `main` equals local `HEAD`. VERIFIED.
- Live github.com and GHE fetch: NOT RUN (manual, G-4).

**F2 (new, go-git bug):** with the transport cache enabled (`ClientOptions.CacheMaxEntries > 0`, which `githttp.NewClient` sets by default), a non-`*http.Transport` `RoundTripper` plus any per-call TLS option (`InsecureSkipTLS`, `CABundle`, client cert, proxy) **panics** at `plumbing/transport/http/common.go:323` (`c.client.Transport.(*http.Transport).Clone()`, unchecked assertion). With `CacheMaxEntries: 0` it returns a clean error `expected underlying client transport to be of type: *http.Transport`. Mitigation: the custom client must wrap an `*http.Transport` (hook `DialContext`/`Proxy`, not a wrapping `RoundTripper`), or set cache 0. Log: `credential_test.go:245` and `:251`.

**F5 (new):** `client.Protocols` is a plain map mutated by `InstallProtocol` with no lock (`plumbing/transport/client/client.go:15,25`). Install once at startup, never per call; there is no per-call transport override in v5.19.2.

### AC2b: cross-host redirect does not forward `Authorization`

- T-REDIRECT `TestRedirect_should_NotForwardAuthorization_When_HostnameDiffers`: server A (`127.0.0.1`, Basic auth) 302s `info/refs` to server B (`localhost:<port>`). B received 0 Authorization headers with both stock and custom client (`A.withAuth=1 B.withAuth=0`). net/http strips it itself when the hostname differs. VERIFIED.
- `TestRedirect_should_ForwardOnlyWithStockClient_When_SameHostnameOtherPort`: A and B are both `127.0.0.1`, different ports.
  - **Stock go-git v5.19.2: `A.withAuth=1 B.withAuth=1`, i.e. the credential IS forwarded** (net/http `shouldCopyHeaderOnRedirect` compares `Hostname()` only, ports ignored; go-git's `ModifyEndpointIfRedirect`, `http/common.go:361`, drops `s.auth` only afterwards, for later requests).
  - **Custom `CheckRedirect` (`StripAuthOnHostChange`, deletes `Authorization`/`Cookie`/`Proxy-Authorization` when `host:port` differs from `via[0]`): `B.withAuth=0`.** VERIFIED.
- **F1:** the plan's redirect test (A to B) must include a same-hostname different-port case or it misses the one real gap; `StripAuthOnHostChange` plus host-scoped `HostAuth.SetAuth` is the policy. This is the CVE-2026-41506 path in ADR-005, narrowed: different-hostname redirects were already safe; same-host-other-port is not. Whether the CVE is exactly this case is INFERRED, not checked against the advisory.

### AC3: HTTPS-to-SSH fallback (ADR-005, plan Story 3.1.3)

`fallback.go` `Fetcher`: on `IsAuthFailure`, only if `Fallback` is true, derive an SSH URL (`SSHURL`: `git@host:path`, no userinfo, no port carried over) and call `fetchSSH` once with `NewSSHAgentAuth("git")`. Counters `HTTPAttempts`, `SSHAttempts`, `Rejects`. Rejected helper-derived credentials get `erase`; Go-native token sources are not erased.

| Test | Result |
|---|---|
| `TestFallback_should_RetryOnceViaSSH_When_SettingTrue_AndNeverSendTokenToSSHHost` | HTTP 401 with a dummy `ghp_...` token, then one SSH fetch succeeds; `HTTPAttempts=1 SSHAttempts=1`; SSH server saw 0 password attempts, 1 exec, no token material in the SSH user; fetched refs present. PASS |
| `..._NotRetryOverSSH_When_SettingFalse` | auth error returned, `SSHAttempts=0`, SSH server saw 0 connections. PASS |
| `..._StopAfterOneSSHAttempt_When_SSHAlsoFails` | both fail, exactly `1/1` attempts (error: `ssh: unable to authenticate, attempted methods [none publickey]`). PASS |
| `..._NotRetry_When_FailureIsNotAuth` | missing repo gives `repository not found`, no SSH attempt. PASS |
| `TestFetch_should_UseSSHURL_When_InsteadOfRewritesHTTPS` | resolver rewrites the HTTP remote to `ssh://git@127.0.0.1:<port>/`; `HTTPAttempts=0`, HTTP server saw 0 requests. PASS |

The token cannot reach the SSH host structurally: the SSH attempt uses a different `AuthMethod` type (agent) and a URL with no userinfo; the test asserts the observable side (no password auth, no token in SSH user, and the HTTP server, not the SSH server, saw the Authorization header). The SSH server's wire bytes are encrypted, so "token never on the wire to the SSH host" is argued by construction, not by capture. VERIFIED (counts), INFERRED (wire).

Caveat for the design: a 404 for a private repo on GitHub is reported by go-git as `ErrAuthenticationRequired` only when the server answers 401; GitHub's actual status for a bad token on a private repo was not exercised (no live auth), so the trigger set (`401`/`403` only) may need `repository not found` added for github.com. UNVERIFIED.

### AC4: `insteadOf`

`rewrite.go` `Rewriter`: reads system/global/repo files in order, follows `[include] path`, reports `includeIf` as unsupported, keeps every `insteadOf`/`pushInsteadOf` value, longest prefix wins, push uses `pushInsteadOf` else `insteadOf`.

- `TestRewriter_should_MatchGitOracle_ForInsteadOfAndPushInsteadOf`: 6 URLs x fetch/push compared with `git remote get-url [--push]` (test-only oracle) under `GIT_CONFIG_GLOBAL`, covering longest-prefix and a base with two `insteadOf` values. 12/12 equal. VERIFIED.
- `TestGoGit_should_OnlyApplyInsteadOfFromRepoLocalConfig_NotGlobal`:
  - repo-local `insteadOf` is applied by go-git on open (`config/config.go:374`, `applyURLRules`): remote URL reads `git@github.com:x/y.git`. VERIFIED.
  - global-only `insteadOf`: go-git URL stays `https://github.com/x/y.git`, git says `git@github.com:x/y.git`. VERIFIED. go-git's `config.Paths(GlobalScope)` lists `$XDG_CONFIG_HOME/git/config` (if set), `~/.gitconfig`, `~/.config/git/config`, ignores `GIT_CONFIG_GLOBAL`, and `LoadConfig` returns only the first file found (`config/config.go:173-199`).
  - go-git's `ReadConfig` does not follow `[include]` (`len(URLs)==0` for an included rule); our resolver does. Multi-valued `insteadOf` is not representable in go-git's `URL` struct (one `InsteadOf` string, `config/url.go:20`); resolver reads `Options.GetAll`.
- **F6:** ADR-005's "resolver-based rewrite" is required and sufficient; `includeIf` is the one unsupported construct (route to CLI via the preflight).

### AC5: ssh_config, known_hosts, agent (what the plan assumes)

All against the in-process SSH server with `HOME` pointed at a temp dir.

| Behaviour | Result | Test |
|---|---|---|
| Agent auth works | pubkey from an in-process agent authenticates | `TestSSH_should_UseAgentAndKnownHosts_Matrix` |
| Unknown host | `knownhosts: key is unknown` | same |
| Mismatched host key | `knownhosts: key mismatch` | same |
| Plain and **hashed** (`ssh-keygen -H`) `~/.ssh/known_hosts` entry | accepted | same |
| No `SSH_AUTH_SOCK` | `error creating SSH agent: "SSH agent requested but SSH_AUTH_SOCK not-specified"` | same |
| `ssh://host/...` with no user | authenticates as the **OS user** (`tstapler`), not `git` (`NewSSHAgentAuth("")`, `ssh/auth_method.go:185`) | same |
| `Hostname`/`Port` alias | honoured (`ssh/common.go` `doGetHostWithPortFromSSHConfig`); known_hosts is checked against the **resolved** `host:port`; an entry keyed by the alias is rejected (OpenSSH would check the alias) | `..._HonourHostnamePort...` |
| `User` in ssh_config | ignored (server saw `git`) | same |
| `StrictHostKeyChecking no` | ignored (still `key is unknown`) | same |
| `IdentityFile` | ignored: server accepts only the IdentityFile key (not in agent) and go-git fails with `unable to authenticate`; an explicit `NewPublicKeysFromFile` auth (what the resolver would build from the IdentityFile) succeeds | `..._IgnoreIdentityFile_AndProxyCommand` |
| `ProxyCommand` | ignored: go-git dials the config `Hostname:Port` directly (`dial tcp 127.0.0.1:1: connection refused`) | same |

`sshprobe.go` `ProbeSSHConfig(path, host)` lists directives go-git ignores. `TestSSHProbe_should_ListUnsupportedDirectives_When_ProxyCommandIdentityFileInclude`: `proxied -> [ProxyCommand]`, `jumped -> [ProxyJump]`, `keyed -> [IdentityFile User]`, `included -> [IdentityFile]` (found through `Include`, which kevinburke/ssh_config v1.2.0 follows), `plain -> []`; `ssh -G -F` confirms OpenSSH applies each of those. VERIFIED. Probed keys: ProxyCommand, ProxyJump, IdentityFile, IdentityAgent, CertificateFile, UserKnownHostsFile, GlobalKnownHostsFile, StrictHostKeyChecking, HostKeyAlias, User, CanonicalizeHostname, ControlPath, KnownHostsCommand, AddKeysToAgent. This is the G5 input for ADR-006's preflight.

**F3 (new, silent failure):** a `Match` block makes kevinburke/ssh_config's `Decode` fail (`Match directive parsing is unsupported`), and `UserSettings.Get` then returns `""` for **every** key (`config.go:75-81`, error swallowed). With go-git that silently drops `Hostname`/`Port` aliases for the whole file. The probe reports such a file as `["Match"]`; the preflight must treat that as "route to CLI" for every SSH host, not only the matched one. Reproduced with a fresh `ssh_config.UserSettings{}` (same type/code path as go-git's `DefaultSSHConfig`) and `HOME` set to a temp dir; `ssh -G` applies the `Match` block. VERIFIED. The tests use a per-file `cfgReader` for isolation because `DefaultUserSettings` caches its first read via `sync.Once`.

### AC6: local `file://` transport spawn behaviour

- `TestFileTransport_should_SpawnGitUploadPack_NotGit_When_UploadPackOnPATH`: shim log is exactly `["git-upload-pack <repo>"]`. go-git (`plumbing/transport/file/client.go:75-95`, `execabs.LookPath`) execs `git-upload-pack` found on PATH; no `git` spawn. VERIFIED.
- `TestFileTransport_should_SpawnGitExecPath_When_UploadPackNotOnPATH` (PATH has only the `git` shim): shim log is `["git --exec-path"]`, then go-git execs `<exec-path>/git-upload-pack` by absolute path. VERIFIED.
- **F7 (the plan's "zero git spawn" is false for `file://` with the default transport):** both variants spawn a git binary (`git-upload-pack` is a link to `git`; `git --exec-path` in the fallback). `PlainClone` of a plain local path takes the same route.
- Fix, VERIFIED: `client.InstallProtocol("file", server.NewClient(server.NewFilesystemLoader(osfs.New("/"))))` (public `plumbing/transport/server`) serves upload-pack and receive-pack in process. `TestFileTransport_should_SpawnNothing_When_InProcessServerInstalledForFile`: clone plus push back through it, remote `main` equals local `HEAD`, shim log empty. Not checked: hooks (the in-process receive-pack runs none), concurrency with a CLI writer holding `index.lock`, `shallow`/protocol-v2 parity, large repos. Treat as a candidate for local ops, not an adopted design.
- `git://` (daemon) was not tested; the product has no use for it (ADR-005 context).

## Decisions for the plan (no fork patch needed)

1. Ship ADR-005 as written, with F1 (`CheckRedirect` strips credentials on `host:port` change and tests include same-host-other-port), F2 (wrap `*http.Transport`, never a bare `RoundTripper`, or cache 0) and F5 (install once) added to Story 3.1.
2. `ResolveHelper` must search known libexec dirs (F4) and never spawn `git --exec-path`.
3. Preflight (ADR-006): route to CLI on `ProxyCommand`, `ProxyJump`, `IdentityAgent`, `Match` (anywhere), `includeIf`; resolve `IdentityFile`/`User`/`StrictHostKeyChecking`/`UserKnownHostsFile`/`HostKeyAlias` in our own code or route to CLI. Pass the OS-user default (F8 below) explicitly.
4. Fix the "no git spawn" wording for local transport (F7) or adopt the in-process `file` server after its own gate.

**F8:** user-less `ssh://` URLs use the OS user; set `git` explicitly (the scp-like `git@host:path` form already does).

## Not verified (named gaps)

- Real github.com and GHE fetch/push, real keychain entries, real `gh` login (forbidden here; manual G-4).
- Real OpenSSH server and real `ssh-agent`/1Password agent (the server and agent are in-process Go stand-ins); `IdentityAgent` handling.
- `GitHub` status codes for bad token versus missing repo (affects the fallback trigger set).
- Windows paths and `file://` drive letters (go-git has `adjustPathForWindows`).
- Wire-level proof that no HTTPS token reaches the SSH host (argued structurally).
- Proxy, `http.proxy`, client-cert, `CABundle` options beyond the F2 panic.
