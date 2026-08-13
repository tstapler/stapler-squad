# UX Research: User-Extensible Agent Detector Plugins (TOML)

Source requirements: `project_plans/detector-plugins/requirements.md`.

## 1. Comparable UX patterns — "user config extends built-in behavior"

| Product | Authoring surface | What makes it easy | What makes it frustrating |
|---|---|---|---|
| **ESLint custom rules / `.eslintrc`** | JS/JSON, schema-validated | `--print-config` shows resolved config; rule docs link directly from lint output (`no-unused-vars` → URL) | A typo in a rule name is silently ignored in some configs; conflicting rule precedence across extends chains is opaque without `--print-config` |
| **Prometheus/Grafana dashboards-as-code** | YAML/JSON | `promtool check config` / `check rules` gives file+line+field errors *before* the process starts serving; Grafana provisioning logs one line per failed dashboard, others still load | Validation errors sometimes only in daemon logs, not the UI — user has to go find them |
| **VS Code `settings.json` / snippets** | JSONC | Inline schema validation *in the editor* (red squiggly + hover message) at authoring time, before save; unknown keys are flagged, not silently dropped | Snippets have no live authoring feedback — only discover a bad snippet when it fails to expand at use time, mirrors the "why didn't my regex fire" problem |
| **git hooks (`.git/hooks/*`)** | Shell scripts | None — no discoverability, no validation, silent no-op if not executable | Textbook worst case: no error surfaced at all when a hook is malformed or not chmod +x. Cited here as the anti-pattern to avoid |
| **fail2ban jails (`jail.local`)** | INI | `fail2ban-client -d` dumps effective config; `fail2ban-client status <jail>` shows match count | Regex failures land only in a syslog line the user must know to grep for |
| **systemd user units (`~/.config/systemd/user/`)** | INI | `systemd-analyze verify unit.service` validates before enabling; `systemctl status` surfaces last failure inline with a timestamp | Drop-in override precedence (`.d/` dirs) is a common source of "why isn't my override applied" confusion — no single command shows final merged unit without `systemctl cat` |
| **shell `rc.d` drop-in dirs** | Shell snippets | Convention (numeric prefix = order) is simple once known | Zero validation, zero feedback; a broken snippet can silently no-op or, worse, break the whole shell startup with no isolation between files |

### Synthesis — what separates "easy" from "frustrating"

1. **Validate before/at load, not just at use time.** ESLint, Prometheus, and systemd all have a "check this config" story that fires immediately, either as a dedicated command (`promtool check`, `systemd-analyze verify`) or inline at authoring time (VS Code schema validation). Detector plugins here validate at startup/hot-reload already per requirement #3 — the gap is *where the result is surfaced* (see §2).
2. **One bad file must not take down the others.** Prometheus/Grafana provisioning and this feature's own requirement #3 ("log-and-skip, not fail-fast") both converge on per-file isolation — already correctly scoped in the requirements.
3. **A "show me the resolved/effective state" view is the single highest-leverage feature.** `--print-config`, `fail2ban-client status <jail>`, `systemctl cat` — every mature system in this category eventually grows a command that answers "what did the system actually load, and did my file take effect." git hooks and rc.d are the negative case precisely because they never grew this. **stapler-squad already has the equivalent for the built-in detector pipeline**: `DetectionEventsPanel.tsx` (`web-app/src/components/sessions/DetectionEventsPanel.tsx`), gated behind `?debug=1`, shows a live table of the last 20 detection events with `matchedPattern`, `matchedCategory`, and `resultStatus` per session, backed by `GetDetectionEvents` RPC. This is the direct analog of `fail2ban-client status` / `--print-config` and should be the reused surface for plugin detectors — no new UI concept is needed, just confirmation that plugin-sourced matches also populate `matchedPattern`/`matchedCategory` the same way built-ins do.

## 2. User mental models: where do they expect to see failures?

Grep across `web-app/src` (`toast|notification|error.?banner|useToast`) shows this repo's existing convention for surfacing backend problems to the user is a **dedicated inline error banner component per feature**, not a global toast system:

- `web-app/src/components/backlog/TriageErrorBanner.tsx` — `role="alert"`, renders inline above the affected content, pairs the message with recovery actions (`onReload`, `onSkip`) rather than just dismissing.
- `web-app/src/components/workflows/WorkflowForm.tsx` — local `error` state (`useState<string | null>`) rendered via `styles.errorBanner` inline in the form, populated either from client-side validation (e.g. cron expression) or from a caught RPC error's `.message`.
- `web-app/src/components/rules/RuleBuilderForm.tsx` — same local-state-inline-banner pattern for another user-authored-config surface (notification rules).

**Implication for this feature:** there is no existing precedent for a global "config problem" banner, and none should be invented. The expected mental model, per this repo's own conventions, is:

- **Logs are the source of truth for the file-level failure** (requirement #3's "clear, actionable error... which file, which field, why" — this satisfies the fail2ban/systemd-journal model, and is consistent with `git hooks`/`rc.d` users who are already comfortable checking logs for a file they hand-authored).
- **A UI surface is only warranted if/when this repo grows a "Detectors" settings section** (see the `SETUP` page's `sectionGroup`/`section` pattern in `web-app/src/app/settings/page.tsx`) — at that point, follow the `TriageErrorBanner`/`WorkflowForm` local-inline-banner convention exactly: a `role="alert"` banner scoped to the affected detector's list row, not a global toast.
- **Per-session confirmation** ("is my custom detector active for *this* session") is already solved by the `DetectionEventsPanel` precedent — extending it to show which detector source (built-in vs. plugin, and which plugin file) produced `matchedPattern` closes the loop the requirements imply ("confidence that a hand-authored regex file 'just works'").

No existing global toast/notification-center primitive should be repurposed for detector validation errors — `NotificationContext.tsx` / `NotificationPanel.tsx` are scoped to session lifecycle events (approvals, review-queue, push notifications), a semantically different domain; conflating "your regex is broken" with "your agent needs approval" would be a mental-model mismatch for the user.

## 3. Accessibility

Confirmed: this phase is backend-only (TOML loader + hot-reload watcher), so no UI surface ships with it — accessibility is N/A for the initial scope, consistent with the requirements' "Out of Scope" framing.

For future phases, if a "Detectors" list/settings view or an extended `DetectionEventsPanel` (showing plugin source) is built, this repo's existing conventions to follow are:

- `role="alert"` on any inline validation/error banner (per `TriageErrorBanner.tsx`), so screen readers announce it immediately without requiring focus.
- `:focus-visible` styling is already defined globally (`web-app/src/app/globals.css` lines 188, 194) — any new interactive element (e.g. a "reload detector" or "open file" button) gets this for free via existing CSS, no new work needed.
- The skip-link / `#main-content` landmark pattern (`web-app/src/app/layout.tsx` lines 64–65) already wraps all page content — a new settings section inherits this automatically.
- Per `.claude/rules/css-architecture.md`, any new component ships as `.css.ts` (vanilla-extract) using `vars.*` tokens, not raw `var(--x)` strings — note `DetectionEventsPanel.tsx` currently uses inline `style={{...}}` with raw `var(--border-color)` etc., which is a pre-existing deviation from the current architecture rule (the component predates the rule or was never migrated) — do not copy that pattern into new plugin-related UI; use `.css.ts` + `vars.*` per the current rule.
- No dedicated `accessibility.md` rule doc exists in `.claude/rules/`; the operative reference is `tests/e2e/accessibility.spec.ts` (Axe Core, run in CI on PRs touching `web-app/src/`) and `.claude/rules/css-architecture.md`'s never-do list (no `position: fixed` without `createPortal`, no hardcoded z-index, etc.) — both apply automatically to any future detector-plugin UI without needing new guidance.

## 4. Error states and edge cases

Mapped against the requirements' validation rules (§3) and the log-and-skip mandate:

| Edge case | Requirement already covers it? | UX handling |
|---|---|---|
| Invalid TOML file present at startup | Yes (req #3) — "log-and-skip" | Startup log line: file path, field, reason. Directory bootstrap (req #6) should log a one-time INFO line noting the directory + example file location so first-run users know it exists at all — this is the "discoverability" gap none of the comparable products solve well (git hooks/rc.d have zero discovery UX) |
| Detector silently not matching due to a typo (e.g. wrong `status` value, or `binary_names` that doesn't match the actual process name) | Partially — req #3 catches unknown `status` values at load time, but a *correct*-schema file with a regex that simply never matches real output is not a "validation error," it's a runtime miss | This is exactly the failure mode `DetectionEventsPanel` exists to diagnose for built-in detectors today (shows `matchedPattern` per event, so "none matched" is visible as an absence). Same panel, extended to tag plugin-origin events, closes this gap without new UI surface. Log-time validation cannot catch this class of bug — it requires the live event view |
| One broken file blocking others | Explicitly required (req #3) — must not fail-fast | No additional UX needed beyond confirming the startup log clearly attributes the skip to the *specific* broken file, not a generic "some detectors failed to load" |
| `id` or `binary_names` collision between two user plugins | Covered by req #3 | Same log-and-skip path; message should name both colliding files, not just the second one loaded |
| Regex catastrophic backtracking / hang | Requirements note Go's RE2 (linear-time) already bounds this — flagged as "confirm during research," a backend/perf question, not a UX one; no user-facing state needed beyond the standard compile-error path if a pattern is rejected for some other reason |
| Hot-reload picks up an edit mid-session | Req #5 | No confirmation UI needed for the happy path (silent pickup is the expected/desired behavior, same as systemd drop-ins or fail2ban `-d`); only the failure path (edit introduces a validation error) needs the same log-and-skip treatment as startup |

## 5. Jobs-to-be-done

- **Functional** — "Get status detection working for my custom/forked agent binary without a stapler-squad rebuild." Directly served by the TOML schema + hot-reload combination; the comparable-products research (§1) confirms this is the same job VS Code snippets, fail2ban jails, and systemd drop-ins all serve — user-authored text extending a host binary's behavior, no compile step.
- **Emotional** — "Confidence that my hand-authored regex file 'just works' without reading Go source." This is the job the `DetectionEventsPanel` precedent is best positioned to serve: a user who drops a TOML file wants fast, visible confirmation their file is live and matching, not just an absence of a startup error. The existing per-session debug panel (`?debug=1`) already answers "is *anything* detecting for this session" — extending its `matchedPattern`/`matchedCategory` columns to show plugin provenance (e.g. a `source: plugin (my-agent.toml)` column) turns "I hope this works" into "I can see it worked," which is the emotional core of the JTBD. Absent that, the fallback (grep the log) mirrors the fail2ban/systemd model and is acceptable but strictly worse.
- **Social** — "Share detector files with the community without needing a PR." This item's Out-of-Scope section explicitly defers remote/distributed manifest fetching (the herdr-style pattern, issue #178) — so sharing today is copy-paste of a `.toml` file between users, same as sharing a `.eslintrc` snippet or a systemd unit file on a forum. The comparable-products research suggests the eventual next step (should this backlog item's remote-fetch follow-up land) is a registry/index model like ESLint's `eslint-config-*` npm packages or Grafana's dashboard marketplace — worth flagging as a forward-looking note for whoever picks up the deferred remote-manifest item, but explicitly out of scope for this research pass.

## Key file references

- `web-app/src/components/sessions/DetectionEventsPanel.tsx` — existing live per-session detector debug view; the primary reusable surface for this feature's UX needs.
- `web-app/src/components/backlog/TriageErrorBanner.tsx`, `web-app/src/components/workflows/WorkflowForm.tsx`, `web-app/src/components/rules/RuleBuilderForm.tsx` — this repo's established "inline, local, `role=alert`" error-banner convention for user-authored-config validation failures; the pattern to follow *if* a detector-plugin UI surface is ever built, in preference to inventing a global toast.
- `web-app/src/app/settings/page.tsx` — `sectionGroup`/`section` structure a future "Loaded Detectors" settings view would slot into.
- `.claude/rules/css-architecture.md` — governs styling for any future plugin-related component (vanilla-extract `.css.ts` + `vars.*`, not inline styles — note `DetectionEventsPanel.tsx` itself predates/deviates from this and should not be copied as a styling example).
- `tests/e2e/accessibility.spec.ts` — CI accessibility gate (Axe Core) any new settings/detector UI would need to pass.
- `project_plans/detector-plugins/requirements.md` — source requirements this research maps against (req #3 validation, req #5 hot-reload, req #6 directory bootstrap).
