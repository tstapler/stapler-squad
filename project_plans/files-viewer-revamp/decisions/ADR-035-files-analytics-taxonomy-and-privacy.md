# ADR-035: Files-viewer analytics taxonomy, privacy rules and typed helper

**Status**: Proposed (2026-10-16)
**Plan**: `implementation/plan.md` Stories 0.2.5, 2.5.3 and the "Analytics" AC line in every firing story; extended by ADR-036 (Task 0.2.5d, "Transport extension" below)
**Builds on**: `project_plans/analytics-system/` (ADR-012 provider adapter, ADR-013 `analytics.db`, ADR-014 ESLint coverage)

## Context
The owner wants long-term understanding of how the Files viewer is used (adoption, conflicts, mobile vs desktop, latency against the SLOs). The platform already exists and was verified by reading source:
- Frontend: `AnalyticsEvent{name, category: user_action|performance|navigation|rpc, durationMs?, sessionId?, page?, component?, labels?: Record<string,string>}` (`web-app/src/lib/analytics/types.ts`), `useAnalytics().track` (`web-app/src/lib/contexts/AnalyticsContext.tsx`), `HttpAnalyticsProvider` batching 25 events / 2 s with a 200-event queue that **drops the oldest** when full (`HttpAnalyticsProvider.ts`).
- Backend: `analytics.AnalyticsProvider.Record(ctx, Event)` (`server/analytics/provider.go`), `POST /api/analytics` with a handler-wide 1000/min limiter, 100 events per batch, and **no label validation** (`server/handlers/analytics_handler.go:140-180`); `GET /api/analytics/summary` returns only the global top 10 events, page views and RPC percentiles, with no label breakdown or name filter.
- Existing naming: both flat snake_case (`backlog_new_item`, `session_deleted`) and dotted `area.action` (`omnibar.create_session`, `file_browser.open_terminal`) exist; `analytics-system/research/pitfalls.md` section 7.2 requires that callers never pass free-form user input as a label and asks for handler-side label validation (not built).
- No generic analytics dashboard page exists (only `/analytics/escape` and the cost-focused `/insights`).

## Decision

### 1. Naming and categories
`files.<area>.<action>` (dotted, lower snake within a segment), the `omnibar.*` precedent. The shared `files.` prefix is what makes `name_prefix` queries possible. Categories: user-initiated actions `user_action`; pane/tab/jumplist movement `navigation`; latencies `performance` with `durationMs`; server-side RPC outcomes `rpc`. `component` is the React component name (`FilesTab`, `SearchPanel`...), `page` is unset (the Files tab is not a route), `sessionId` is the existing opaque session id.

### 2. Privacy rules (hard rules, enforced by types and tests)
Allowed label values: closed enums, booleans (`"true"`/`"false"`), count/size/length **buckets**, extension **class** buckets, form factor, input profile, flag state. Forbidden anywhere in an event (name, labels, `component`, `page`): file paths, directory names, file names, extensions as such, file contents, query text, glob text, branch names, session titles, draft text, error message strings, git refs.
- Every label value is typed as a string-literal union produced by a helper (`bucketCount`, `bucketBytes`, `bucketQueryLen`, `extClass(path)` which returns only the class and never stores the path). There is no `string`-typed label field in the registry.
- Shape limits (also enforced server-side for `files.*` events in the handler, see Consequences): at most 8 labels per event; key `^[a-z][a-z0-9_]{0,23}$`; value `^[a-z0-9_+-]{1,24}$` (rules out `/`, `.`, spaces, upper case, so paths and file names cannot pass). Shape checks cannot catch a short lowercase query word; the type system and the canary test are the real guard.
- Cardinality: every label has a finite value set declared in the registry (largest set: `ext_class` with 7 values). No unbounded ids; no per-event random correlation ids (the find-and-open metric is measured with a client-side interaction meter, section 5).
- Stamped on every event by the helper (not by call sites): `form` = `mobile|desktop` (the same 768 px breakpoint as the layout), `input` = `touch|hardware` (`InputProfile`), `editing` = `on|off` (`files:editing`), `v2` = `on|off` (`files:viewer-v2`). These are the only dimensions for the mobile/desktop split.

### 3. Event taxonomy
Labels shown per event are in addition to the four stamped labels. `dur` = `durationMs` set. Enum value sets are the TypeScript unions in `web-app/src/lib/files/analytics/events.ts` (single source; docs and tests are generated from it).

| Event | Cat | Labels (enum / bucket) | Notes |
|---|---|---|---|
| `files.search.run` | user_action | `query_len` {2-3,4-8,9-32,33+}, `regex`, `case_sensitive`, `whole_word`, `has_globs`, `include_ignored` (bool) | Fires once per request actually sent (after the 300 ms debounce) |
| `files.search.results` | user_action | `outcome` {ok,empty,truncated,invalid_regex,error,worktree_unavailable,flag_off,busy}, `results` {0,1-9,10-99,100-999,1000+}, `files` {0,1-9,10-99,100+}, `queued` (bool) | Fires on the final `stats` or the error |
| `files.search.open_hit` | user_action | `rank` {1,2-5,6-20,21+}, `interactions` {1,2,3,4,5+}, `via` {tap,enter,quickfix}, `reused_tab` (bool) | `interactions` is the find-and-open meter (section 5) |
| `files.search.cancel` | user_action | `phase` {queued,running}, `results` bucket | |
| `files.search.queued` | user_action | `ahead` {1,2+} | |
| `files.tab.open` | user_action | `kind` {preview,pinned}, `source` {tree,search,palette,recent,deeplink,outline,jumplist,restore}, `ext_class` {code,markup,config,doc,data,other,none}, `size` {lt10k,10k-100k,100k-1m,gt1m}, `open_tabs` {1,2-3,4-9,10+} | |
| `files.tab.close` | user_action | `dirty` (bool), `method` {button,keyboard,context_menu,close_others,evict}, `open_tabs` bucket | |
| `files.tab.switch` | navigation | `via` {strip,sheet,jumplist,keyboard,quickfix} | Coalesced within 250 ms |
| `files.tab.evict` | user_action | `outcome` {evicted_clean_preview,refused_all_pinned_or_dirty} | Pinned and dirty tabs are never evicted (ux.md section 5) |
| `files.tab.evict_undo` | user_action | none | Undo after an eviction announcement |
| `files.tab.restore` | navigation | `outcome` {ok,partial,storage_unavailable}, `tabs` bucket | On Files tab mount |
| `files.open.failed` | user_action | `reason` {not_found,too_large,binary,encoding,worktree_unavailable,rpc_error} | Loading-state failure (ux.md section 8.5) |
| `files.nav.back` / `files.nav.forward` | navigation | `via` {button,keyboard,long_press_menu}, `depth` {1,2-5,6+} | |
| `files.nav.jump_line` | navigation | `via` {palette,status_bar,vim}, `clamped` (bool) | |
| `files.nav.outline` | navigation | `outcome` {jumped,empty,unavailable_large,closed}, `symbols` {0,1-9,10-99,100+} | |
| `files.nav.palette` | user_action | `mode` {file,command,line,outline}, `outcome` {run,empty,cancel}, `command` {none,toggle_vim,toggle_wrap,save,reload,shortcuts,goto_line,toggle_accessible_view} | `command` is the fixed palette command id, never typed text |
| `files.edit.enter` | user_action | `via` {button,fab,key,vim_i,read_hint}, `ext_class` | |
| `files.edit.exit` | user_action | `via` {stop_editing,flag_off,tab_close}, `dirty` (bool) | |
| `files.edit.read_hint` | user_action | `outcome` {shown,entered_edit,dismissed} | Typing in Read mode (ux.md section 8.1) |
| `files.edit.save` | user_action | `outcome` {saved,conflict,rejected,unknown,offline_blocked}, `via` {button,shortcut,vim_w,key_bar,top_bar,palette,save_all}, `size` bucket | Latency is `files.perf.save_ms`; `save_all` = the sequential Close-others save (ux.md 11.1a) |
| `files.edit.save_conflict` | user_action | `theirs` {full_text,deleted,too_large,binary,non_utf8,mixed_eol,symlink}, `merge_available` (bool) | |
| `files.edit.save_rejected` | user_action | `reason` {flag_off,truncated,binary,too_large,non_utf8,mixed_eol,symlink,worktree_missing,hidden_session,git_internal,permission,other} | Mirrors `ReadOnlyReason` plus I/O classes |
| `files.edit.merge_open` | user_action | `hunks` {0,1-3,4-10,11+}, `conflicts` {0,1,2-5,6+} | |
| `files.edit.merge_saved` | user_action | `reviewed` (bool), `conflicts_resolved` bucket, `auto_only` (bool) | |
| `files.edit.overwrite` | user_action | `outcome` {confirmed,cancelled} | |
| `files.edit.reload` | user_action | `source` {conflict_dialog,stale_banner}, `copied_first` (bool) | |
| `files.edit.draft_restore` | user_action | `outcome` {restored,restored_conflict,discarded,not_now,missing_file}, `age` {lt1h,1-6h,6-24h} | |
| `files.edit.draft_not_stored` | user_action | `reason` {too_large,storage_full,backup_unavailable} | `backup_unavailable` = the IndexedDB tier (ux.md 11.3) failed or is missing; emitted once per buffer per session |
| `files.stale.shown` | navigation | `dirty` (bool), `actor` {agent_likely,unknown} | `agent_likely` only when the session is in a running state; never asserted as fact (ux.md section 8.7) |
| `files.stale.action` | user_action | `action` {reload,review,dismiss} | |
| `files.vim.toggle` | user_action | `to` {on,off}, `source` {status_bar,overflow,palette,auto_hardware} | |
| `files.vim.escape` | user_action | `via` {esc_esc,status_control,ex_leave}, `mode` {normal,insert,visual,command} | |
| `files.a11y.accessible_view` | user_action | `to` {on,off}, `via` {overflow,palette,skip_link}, `size` bucket | Accessible view toggle (ux.md 8.10, Story 1.5.4); the only signal of assistive-technology use, deliberately not inferred |
| `files.input.profile_change` | navigation | `to` {touch,hardware}, `source` {override,auto_pointer,auto_chord,auto_key,auto_viewport} | Session-only auto-detect flips and manual overrides (ux.md 8.8); a flip to `hardware` on a `form=mobile` event with no later hardware-key use is the detector false-positive signal |
| `files.mobile.layout_fallback` | navigation | `step` {vim_row_merged,banner_folded,top_bar_hidden,status_hidden,floor}, `viewport` {xs_portrait,s_portrait,landscape} | Edit-layout ladder (ux.md 7.1a); coalesced within 2 s; shows how often phones need the fallbacks |
| `files.mobile.pane_switch` | navigation | `from`/`to` {files,editor,search}, `via` {bottom_bar,back_arrow,system_back,tap_hit} | |
| `files.mobile.extra_key_used` | user_action | `key_class` {esc,tab,modifier,arrow,bracket,punct,undo_redo,save,vim_row}, `count` {1,2-5,6-20,21+} | Aggregated per 30 s window per class, never per keystroke |
| `files.mobile.keyboard_open` | user_action | `trigger` {edit_enter,tap}, `bar_visible` (bool) | Coalesced within 2 s |
| `files.perf.open_ms` | performance | `size` bucket, `degraded` (bool) | `durationMs` = RPC-return to first paint (the 300 ms SLO) |
| `files.perf.search_first_result_ms` | performance | `scale` {lt1k,1k-10k,10k-50k,50k+} files scanned bucket, `queued` (bool) | Client-measured; SLO 2000 ms |
| `files.perf.search_total_ms` | performance | `outcome` {ok,truncated,cancelled,error}, `scale` | |
| `files.perf.save_ms` | performance | `outcome` {saved,conflict,rejected}, `size` bucket | |
| `files.meta.dropped` | performance | `event` {the capped event names}, `count` bucket | Emitted by the rate cap (section 4), so lost events are visible |
| `files.server.write` | rpc | `outcome` {ok,conflict_rejected,denied,disabled,error}, `reason` {none, ReadOnlyReason values}, `size` bucket, `eol` {lf,crlf,mixed}, `bom` (bool) | Server; `durationMs` = write duration; same outcome enum as the slog line |
| `files.server.search` | rpc | `outcome` {ok,truncated,cancelled,invalid,error,disabled,busy}, `results` bucket, `scale` bucket, `queued` (bool), `backend` {walker} | Server; `durationMs` = total |
| `files.server.search_first_result` | performance | `scale` bucket | Server first-batch latency, the authoritative SLO series |

Server events carry no `form`/`input` (the server cannot know them); the client events are the form-factor series.

### 4. Typed helper and sampling
One module, `web-app/src/lib/files/analytics/`, is the only code allowed to call `track` with a `files.` name:
- `events.ts`: the registry (`FilesEvents` const object of names; per-event label types as literal unions; bucket functions).
- `filesAnalytics.ts`: `useFilesAnalytics()` wraps `useAnalytics()` and returns `emit(name, labels)` typed by the registry, `time(name)` for performance events, and the stamped context. The hook is memoised (stable identity).
- `sampling.ts`: (a) per-name **token bucket** of 30 events/min (protects the handler-wide 1000/min limit and the provider's 200-event drop-oldest queue); drops are counted and flushed as one `files.meta.dropped`; (b) **coalescer** for `files.tab.switch` and `files.mobile.keyboard_open`; (c) **window aggregator** for `files.mobile.extra_key_used` (one event per key class per 30 s, flushed on `pagehide`). All time is injected (fake-timer tests).
- ESLint: `no-restricted-syntax` override for `web-app/src/components/sessions/**` and `web-app/src/lib/files/**` forbids a `track({ name: "files.` literal outside the helper, so names cannot be hand-typed at call sites.
- Analytics never blocks or throws: `emit` is wrapped so a provider error is swallowed, and Files functionality does not depend on analytics being enabled.

### 5. Success metrics as derived queries
All over `events` in a window, grouped by the stamped `form` label. Raw form (the how-to in Task 2.5.3c carries the SQL against `analytics.db`, path `~/.stapler-squad/<instance>/analytics.db` per ADR-013; the endpoint in Task 2.5.3a computes the same):
- **Find-and-open <= 3 interactions** = `count(open_hit where interactions in {1,2,3}) / count(open_hit)`; target >= 0.9 once the feature is in use. `interactions` comes from a client-side `FindFlowMeter` (interactions counted by the requirements.md protocol: one per tap or chord, typing a query is one): it starts when the search panel opens, increments on each discrete user action until `open_hit`, and is discarded on cancel. It is a usage metric alongside, not a replacement for, the Playwright ship gate.
- **Adoption (per week)** = distinct ISO weeks and counts of saves, searches and Edit entries, plus weekly active days (distinct dates with any `files.*` event); the requirements.md owner-observed adoption check reads this at the 4-week retirement review. **Counting source (triad round 2)**: the client token bucket (30 events/min per name), the 250 ms/2 s coalescers and the provider's 200-event drop-oldest queue (which also loses events while the tab is offline or closing) can only *undercount* client events, so the adoption counts use the unthrottled **server-side** events wherever one exists: saves = `files.server.write{outcome=ok}`, searches = `files.server.search` (all outcomes counted, `ok|truncated` reported separately), and the client `files.edit.enter` / `files.search.run` are used only as the lower-bound cross-check; `files.meta.dropped` totals are shown next to every client-derived count. Edit entries have no server event, so that count is a client lower bound and is labelled as such. At the volumes of one user (tens of events a day) the 30/min cap is not expected to bind; the displayed `dropped` count proves it.
- **Adoption review rule (proposed constants, owner-editable)** at the 4-week `files:viewer-v2` review, over the trailing 28 days, with the verdict computed by the endpoint (Task 2.5.3a) from named constants `adoptionKeepActiveDays = 8`, `adoptionKeepSearches = 20`, `adoptionEditSaves = 5`, `adoptionMinEvents = 150`, `adoptionMinActiveDays = 6`:
  - **Inconclusive** (extend the observation by 4 weeks once, then decide on the owner's own report): fewer than `adoptionMinEvents` server-side events or fewer than `adoptionMinActiveDays` active days in the window. One user on a feature they may simply not need yet produces small numbers; below this volume a ratio such as the conflict rate is noise and is not shown as a percentage.
  - **Keep v2 and retire the kill switch** (backlog follow-up from Story 2.5.1): active days >= 8 **and** searches >= 20 **and** no `files:viewer-v2` rollback in the window.
  - **Editing proven** (leave `files:editing` on, no further editing investment): saves (`ok`) >= 5 **and** a save-conflict rate under 0.25 or at least 20 saves.
  - **Editing tried but not adopted** (investigate friction before more editing work): Edit entries >= 5 **and** saves < 2.
  - **Retire or rework** (v2 only): meets `adoptionMinEvents` but active days < 4 **and** searches < 5, i.e. used and dropped.
  These are proposals: the owner edits the five constants (one Go file, one place) before the review; the thresholds are a decision aid, not a ship gate.
  - **Rationale and sourcing of the constants (triad round 3; owner-editable proposals, not measurements)**: no external benchmark exists for the adoption of a one-user tool, and none was found or claimed, so the values are the author's judgment (INFERRED): the 28-day window is four weekly cycles so one busy or idle week cannot decide; `adoptionMinActiveDays = 6` is about 1.5 days a week, below which the owner's real usage pattern has not yet shown up; `adoptionMinEvents = 150` is about 25 server events on each of those 6 days, i.e. several real sessions rather than one (a single focused session already produces dozens of searches and saves); `adoptionKeepActiveDays = 8` is every other working day of the 20 in the window; `adoptionKeepSearches = 20` is about one search per working day; `adoptionEditSaves = 5` is a little over one save a week. The review rule is to **edit the constants before looking at the verdict**, to avoid fitting them to the answer; after two weeks of real data the owner can re-read the raw counts (the endpoint shows them) and adjust once. The comparator for the find metric is the owner's measured current time-to-answer on the phone (plan Task 0.1.5d), not a target taken from elsewhere.
  - **Action by verdict (triad round 3)**: *inconclusive* -> extend once by 4 weeks, then decide on the owner's own report; *keep_v2* -> retire the kill switch and the legacy layout (Story 2.5.1 backlog item); *editing_proven* -> leave `files:editing` on; *editing_tried_not_adopted* or *retire_or_rework*, or *inconclusive* twice -> **low-adoption path**: (1) flip the global `files:editing` flag off (no code removal; `WriteFile` then refuses everything), (2) **keep the read-only features** (search, tabs, mobile read mode, Accessible view): their cost is already spent and they carry no write-path risk, (3) open a backlog item to delete the write path, 3-way merge, key bar and draft stores if editing is still unused after a further 8 weeks, (4) only if search and tabs are also below the `retire_or_rework` thresholds, retire v2 by removing the new layout and keeping the legacy single-file tab. The decision and its date are recorded on the backlog item that Story 2.5.1 creates.
- **Save-conflict rate** = `count(save{outcome=conflict}) / count(save)`; also `count(files.edit.overwrite{confirmed}) / count(save_conflict)` (how often the owner overwrites rather than merges/reloads) and merge completion = `merge_saved / merge_open`.
- **Mobile vs desktop split** = share of `files.search.run`, `files.edit.save`, `files.tab.open` by `form`; `input` splits touch vs hardware keyboard on mobile.
- **Search latency vs 2 s SLO** = p50/p95 of `files.perf.search_first_result_ms` and server `files.server.search_first_result`, share `<= 2000 ms`, split by `scale`.
- **Open latency vs 300 ms SLO** = p50/p95 of `files.perf.open_ms`, split by `size` and `degraded`.
- **Agent-race exposure** = `files.stale.shown` per active day, split by `actor` and `dirty`.

### 6. Server-side events
`FileWriter` and the search handler get a `FilesTelemetry` port (`server/services/files_telemetry.go`): typed `WriteOutcomeEvent` / `SearchOutcomeEvent` structs with enum-typed fields (no `string` path or query field exists on them), a no-op default, and an adapter that maps them to `analytics.Event` and calls `AnalyticsProvider.Record` synchronously after the response is decided (one insert into the separate `analytics.db`, ADR-013), with a 500 ms detached context; failure logs at Warn and never alters the RPC result. It complements, not replaces, the `slog` lines (slog keeps `path`; analytics never does).
### 7. Transport extension (ADR-036; added 2026-10-16)
New closed-enum labels (all subject to the shape rules of section 2; none is a path, offset, line, byte count, text or query): `transport` {full,connect_unary,connect_stream,ws_stream,http_range}; `fallback_reason` {network,unavailable,deadline,malformed,capability,version}; `payload` (bytes of the request or response body) {lt1k,1k-10k,10k-100k,100k-1m,ge1m}; `ops` (edits in one patch) {1,2-9,10-99,100-999,1000+}; `tier` {t0,t1,t2}; `op` {read,patch,stat,write_full}; `patch_reason` {offset_mismatch,result_mismatch,out_of_order,analysis_pending,patch_too_large,diverged}. The existing `size` bucket is unchanged (`gt1m` covers T1/T2); `files.search.run` gains `large_files` (bool) and `files.server.search` gains `large` (bool).

| Event | Cat | Labels | Notes |
|---|---|---|---|
| `files.transport.select` | navigation | `op`, `transport`, `reason` {capabilities,flag,pinned_full,version} | Once per page per op change |
| `files.transport.fallback` | performance | `op`, `from`, `to` (`transport` values), `fallback_reason` | After three consecutive transport-class failures; once per demotion |
| `files.read.window` | performance | `transport`, `payload`, `outcome` {ok,torn,stale,error,aborted}, `cache` {hit,miss}, `direction` {around,fwd,back} | Client, sampled by the section 4 token bucket; `durationMs` set |
| `files.edit.patch_rejected` | user_action | `patch_reason`, `tier`, `transport` | Client; no offsets |
| `files.edit.preflight` | user_action | `outcome` {ok,lines_moved,mismatch,analysis_pending,error} | Same-position rebase attempt (ADR-034 amendment) |
| `files.server.patch` | rpc | `outcome` {ok,conflict,offset_mismatch,result_mismatch,out_of_order,analysis_pending,too_large,rejected,disabled,error}, `tier`, `transport`, `payload`, `ops` | Server, unthrottled, mirrors `files.server.write`; the same outcome enum as the slog line; `durationMs` set |

`files.edit.save` additionally carries `transport`, `payload` and `ops` when the save went through the transport port, and `files.perf.open_ms` additionally carries `tier`. Section 5 gains the derived series described in plan Story 2.5.3 (adapter mix, fallback rate, patch versus whole-file save share, median payload bucket, patch rejection rate by reason). The server port gains `RecordPatch` (enum-typed `PatchOutcomeEvent`, no path or offset field).


## Consequences
- Adds a typed registry that must be kept the single source: the ADR table, the tests and the how-to are checked against `events.ts` by a jest test, so docs cannot drift.
- Requires two small additions to the shared analytics platform, both tracked in plan tasks: (a) a `files.`-scoped label-shape guard in `analytics_handler.go` that drops (not 400s) violating events and logs a count without values; (b) a prefix-filtered, label-grouped summary (`GET /api/analytics/files`), because the generic `/api/analytics/summary` returns the global top 10 only.
- The label-shape guard is defense in depth, not the privacy boundary; the boundary is "no free-form string reaches `labels`", enforced by types and the canary test.
- `HttpAnalyticsProvider` drop-oldest behavior means a burst can lose events; the rate cap plus `files.meta.dropped` makes that visible instead of silent.
- Single-user tool: the data never leaves the machine (`analytics.db`), consistent with the platform's offline NFR. Retention follows the platform's 90 days / 100k events; the 4-week review fits inside it.

## Alternatives rejected
- Hand-typed `track({name: "files...."})` at call sites: drifts, no label typing, invites a path or query into labels.
- Logging only (slog) with no analytics events: no frontend form-factor or interaction data, no long-term store.
- A new Files-specific events table: duplicates `AnalyticsEvent`; the JSON `labels` column was designed for this (ADR-013).
- Hashing paths or queries into labels: still a stable per-file/per-query identifier and an enumeration risk; counts and classes answer every metric here.
- Sampling the whole feature: volumes are tiny for a single user; only genuinely high-frequency events are capped.
