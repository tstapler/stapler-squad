// Single source of truth for Files analytics (ADR-035). The ADR tables are checked against this
// registry by __tests__/events.test.ts, so edit both together.
import type { AnalyticsEvent } from "@/lib/analytics/types";

type Category = AnalyticsEvent["category"];
type Labels = Record<string, readonly string[]>;

const BOOL = ["true", "false"] as const;
const SIZE = ["lt10k", "10k-100k", "100k-1m", "gt1m"] as const;
const TABS = ["0", "1", "2-3", "4-9", "10+"] as const;
const RESULTS = ["0", "1-9", "10-99", "100-999", "1000+"] as const;
const FILES = ["0", "1-9", "10-99", "100+"] as const;
const SCALE = ["lt1k", "1k-10k", "10k-50k", "50k+"] as const;
const CONFLICTS = ["0", "1", "2-5", "6+"] as const;
const EXT_CLASS = ["code", "markup", "config", "doc", "data", "other", "none"] as const;
const TRANSPORT = ["full", "connect_unary", "connect_stream", "ws_stream", "http_range"] as const;
const TIER = ["t0", "t1", "t2"] as const;
const PAYLOAD = ["lt1k", "1k-10k", "10k-100k", "100k-1m", "ge1m"] as const;
const OPS = ["1", "2-9", "10-99", "100-999", "1000+"] as const;
const OP = ["read", "patch", "stat", "write_full"] as const;
const SAVE_REASON = [
  "flag_off", "truncated", "binary", "too_large", "non_utf8", "mixed_eol", "symlink",
  "worktree_missing", "hidden_session", "git_internal", "permission", "other",
] as const;
const SERVER_OUTCOME = ["ok", "truncated", "cancelled", "invalid", "error", "disabled", "busy"] as const;
const BIT = ["0", "1"] as const;

/** The six search options packed into one label (regex, case, whole word, globs, ignored, large) to stay inside the 8-label cap. */
export type SearchOpts = `r${(typeof BIT)[number]}c${(typeof BIT)[number]}w${(typeof BIT)[number]}g${(typeof BIT)[number]}i${(typeof BIT)[number]}l${(typeof BIT)[number]}`;

const SEARCH_OPTS = BIT.flatMap((r) => BIT.flatMap((c) => BIT.flatMap((w) => BIT.flatMap((g) => BIT.flatMap((i) => BIT.map((l) => `r${r}c${c}w${w}g${g}i${i}l${l}` as SearchOpts))))));

const ev = <C extends Category, const L extends Labels>(category: C, labels: L) => ({ category, labels });

const BASE_EVENTS = {
  "files.search.run": ev("user_action", { query_len: ["2-3", "4-8", "9-32", "33+"], opts: SEARCH_OPTS }),
  "files.search.results": ev("user_action", {
    outcome: ["ok", "empty", "truncated", "invalid_regex", "error", "worktree_unavailable", "flag_off", "busy"],
    results: RESULTS, files: FILES, queued: BOOL,
  }),
  "files.search.open_hit": ev("user_action", {
    rank: ["1", "2-5", "6-20", "21+"], interactions: ["1", "2", "3", "4", "5+"],
    via: ["tap", "enter", "quickfix"], reused_tab: BOOL,
  }),
  "files.search.cancel": ev("user_action", { phase: ["queued", "running"], results: RESULTS }),
  "files.search.queued": ev("user_action", { ahead: ["1", "2+"] }),
  "files.tab.open": ev("user_action", {
    kind: ["preview", "pinned"],
    source: ["tree", "search", "palette", "recent", "deeplink", "outline", "jumplist", "restore"],
    ext_class: EXT_CLASS, size: SIZE,
  }),
  "files.tab.close": ev("user_action", {
    dirty: BOOL, method: ["button", "keyboard", "context_menu", "close_others", "evict"], open_tabs: TABS,
  }),
  "files.tab.switch": ev("navigation", { via: ["strip", "sheet", "jumplist", "keyboard", "quickfix"] }),
  "files.tab.evict": ev("user_action", { outcome: ["evicted_clean_preview", "refused_pinned_dirty"] }),
  "files.tab.evict_undo": ev("user_action", {}),
  "files.tab.restore": ev("navigation", { outcome: ["ok", "partial", "storage_unavailable"], tabs: TABS }),
  "files.open.failed": ev("user_action", {
    reason: ["not_found", "too_large", "binary", "encoding", "worktree_unavailable", "rpc_error"],
  }),
  "files.nav.back": ev("navigation", { via: ["button", "keyboard", "long_press_menu"], depth: ["1", "2-5", "6+"] }),
  "files.nav.forward": ev("navigation", { via: ["button", "keyboard", "long_press_menu"], depth: ["1", "2-5", "6+"] }),
  "files.nav.jump_line": ev("navigation", { via: ["palette", "status_bar", "vim"], clamped: BOOL }),
  "files.nav.outline": ev("navigation", {
    outcome: ["jumped", "empty", "unavailable_large", "closed"], symbols: FILES,
  }),
  "files.nav.palette": ev("user_action", {
    mode: ["file", "command", "line", "outline"], outcome: ["run", "empty", "cancel"],
    command: ["none", "toggle_vim", "toggle_wrap", "save", "reload", "shortcuts", "goto_line", "toggle_accessible_view"],
  }),
  "files.edit.enter": ev("user_action", { via: ["button", "fab", "key", "vim_i", "read_hint"], ext_class: EXT_CLASS }),
  "files.edit.exit": ev("user_action", { via: ["stop_editing", "flag_off", "tab_close"], dirty: BOOL }),
  "files.edit.read_hint": ev("user_action", { outcome: ["shown", "entered_edit", "dismissed"] }),
  "files.edit.save": ev("user_action", {
    outcome: ["saved", "conflict", "rejected", "unknown", "offline_blocked"],
    via: ["button", "shortcut", "vim_w", "key_bar", "top_bar", "palette", "save_all"], size: SIZE,
  }),
  "files.edit.save_conflict": ev("user_action", {
    theirs: ["full_text", "deleted", "too_large", "binary", "non_utf8", "mixed_eol", "symlink"], merge_available: BOOL,
  }),
  "files.edit.save_rejected": ev("user_action", { reason: SAVE_REASON }),
  "files.edit.merge_open": ev("user_action", { hunks: ["0", "1-3", "4-10", "11+"], conflicts: CONFLICTS }),
  "files.edit.merge_saved": ev("user_action", { reviewed: BOOL, conflicts_resolved: CONFLICTS, auto_only: BOOL }),
  "files.edit.overwrite": ev("user_action", { outcome: ["confirmed", "cancelled"] }),
  "files.edit.reload": ev("user_action", { source: ["conflict_dialog", "stale_banner"], copied_first: BOOL }),
  "files.edit.draft_restore": ev("user_action", {
    outcome: ["restored", "restored_conflict", "discarded", "not_now", "missing_file"], age: ["lt1h", "1-6h", "6-24h"],
  }),
  "files.edit.draft_not_stored": ev("user_action", { reason: ["too_large", "storage_full", "backup_unavailable"] }),
  "files.stale.shown": ev("navigation", { dirty: BOOL, actor: ["agent_likely", "unknown"] }),
  "files.stale.action": ev("user_action", { action: ["reload", "review", "dismiss"] }),
  "files.vim.toggle": ev("user_action", { to: ["on", "off"], source: ["status_bar", "overflow", "palette", "auto_hardware"] }),
  "files.vim.escape": ev("user_action", {
    via: ["esc_esc", "status_control", "ex_leave"], mode: ["normal", "insert", "visual", "command"],
  }),
  "files.a11y.accessible_view": ev("user_action", { to: ["on", "off"], via: ["overflow", "palette", "skip_link", "toolbar"], size: SIZE }),
  "files.input.profile_change": ev("navigation", {
    to: ["touch", "hardware"], source: ["override", "auto_pointer", "auto_chord", "auto_key", "auto_viewport"],
  }),
  "files.mobile.layout_fallback": ev("navigation", {
    step: ["vim_row_merged", "banner_folded", "top_bar_hidden", "status_hidden", "floor"],
    viewport: ["xs_portrait", "s_portrait", "landscape"],
  }),
  "files.mobile.pane_switch": ev("navigation", {
    from: ["files", "editor", "search"], to: ["files", "editor", "search"],
    via: ["bottom_bar", "back_arrow", "system_back", "tap_hit"],
  }),
  "files.mobile.extra_key_used": ev("user_action", {
    key_class: ["esc", "tab", "modifier", "arrow", "bracket", "punct", "undo_redo", "save", "vim_row"],
    count: ["1", "2-5", "6-20", "21+"],
  }),
  "files.mobile.keyboard_open": ev("user_action", { trigger: ["edit_enter", "tap"], bar_visible: BOOL }),
  "files.perf.open_ms": ev("performance", { size: SIZE, degraded: BOOL, tier: TIER }),
  "files.perf.search_first_result_ms": ev("performance", { scale: SCALE, queued: BOOL }),
  "files.perf.search_total_ms": ev("performance", { outcome: ["ok", "truncated", "cancelled", "error"], scale: SCALE }),
  "files.perf.save_ms": ev("performance", { outcome: ["saved", "conflict", "rejected"], size: SIZE }),
  "files.transport.select": ev("navigation", {
    op: OP, transport: TRANSPORT, reason: ["capabilities", "flag", "pinned_full", "version"],
  }),
  "files.transport.fallback": ev("performance", {
    op: OP, from: TRANSPORT, to: TRANSPORT, fallback_reason: ["network", "unavailable", "deadline", "malformed", "capability", "version"],
  }),
  "files.read.window": ev("performance", {
    transport: TRANSPORT, payload: PAYLOAD, outcome: ["ok", "torn", "stale", "error", "aborted"], cache: ["hit", "miss"],
  }),
  "files.edit.patch_rejected": ev("user_action", {
    patch_reason: ["offset_mismatch", "result_mismatch", "out_of_order", "analysis_pending", "patch_too_large", "diverged"],
    tier: TIER, transport: TRANSPORT,
  }),
  "files.edit.preflight": ev("user_action", { outcome: ["ok", "lines_moved", "mismatch", "analysis_pending", "error"] }),
  "files.server.write": ev("rpc", {
    outcome: ["ok", "conflict_rejected", "denied", "disabled", "error"], reason: ["none", ...SAVE_REASON],
    size: SIZE, eol: ["lf", "crlf", "mixed"], bom: BOOL,
  }),
  "files.server.search": ev("rpc", {
    outcome: SERVER_OUTCOME, results: RESULTS, scale: SCALE, queued: BOOL, backend: ["walker"], large: BOOL,
  }),
  "files.server.search_first_result": ev("performance", { scale: SCALE }),
  "files.server.patch": ev("rpc", {
    outcome: ["ok", "conflict", "offset_mismatch", "result_mismatch", "out_of_order", "analysis_pending", "too_large", "rejected", "disabled", "error"],
    tier: TIER, transport: TRANSPORT, payload: PAYLOAD, ops: OPS,
  }),
} as const;

type BaseName = keyof typeof BASE_EVENTS;
type SlugOverrides = { "files.perf.search_first_result_ms": "perf_search_first_ms" };
export type Slug<N extends string> = N extends keyof SlugOverrides
  ? SlugOverrides[N]
  : N extends `files.${infer A}.${infer B}` ? `${A}_${B}` : never;
/** Short, shape-valid stand-in for an event name in the `event` label of `files.meta.dropped`. */
export type DroppedSlug = Slug<Exclude<BaseName, `files.server.${string}`>>;

export function slugOf<N extends string>(name: N): Slug<N> {
  if (name === "files.perf.search_first_result_ms") return "perf_search_first_ms" as Slug<N>;
  return name.slice("files.".length).replace(".", "_") as Slug<N>;
}

const DROPPED_SLUGS = (Object.keys(BASE_EVENTS) as BaseName[])
  .filter((n) => !n.startsWith("files.server."))
  .map((n) => slugOf(n)) as DroppedSlug[];

export const FILES_EVENTS = {
  ...BASE_EVENTS,
  "files.meta.dropped": ev("performance", { event: DROPPED_SLUGS, count: ["1", "2-5", "6-20", "21+"] }),
} as const;

export type FilesEventName = keyof typeof FILES_EVENTS;
/** Server events are recorded by Go; the browser never emits them. */
export type ClientEventName = Exclude<FilesEventName, `files.server.${string}`>;


export type LabelsOf<N extends FilesEventName> = {
  -readonly [K in keyof (typeof FILES_EVENTS)[N]["labels"]]: (typeof FILES_EVENTS)[N]["labels"][K] extends readonly (infer V)[] ? V : never;
};

/** Own labels the event may carry; the four stamps (form, input, editing, v2) are added by the helper. */
export const STAMP_KEYS = ["form", "input", "editing", "v2"] as const;

// ---- bucket functions: counts in, closed-enum labels out ----

function bucketer<const L extends readonly string[]>(labels: L, upper: readonly number[]) {
  return (n: number): L[number] => {
    const i = upper.findIndex((u) => n <= u);
    return labels[i === -1 ? labels.length - 1 : i];
  };
}

export const bucketQueryLen = bucketer(["2-3", "4-8", "9-32", "33+"], [3, 8, 32]);
export const bucketResults = bucketer(RESULTS, [0, 9, 99, 999]);
export const bucketFiles = bucketer(FILES, [0, 9, 99]);
export const bucketTabs = bucketer(TABS, [0, 1, 3, 9]);
export const bucketRank = bucketer(["1", "2-5", "6-20", "21+"], [1, 5, 20]);
export const bucketInteractions = bucketer(["1", "2", "3", "4", "5+"], [1, 2, 3, 4]);
export const bucketKeyCount = bucketRank;
export const bucketSymbols = bucketFiles;
export const bucketHunks = bucketer(["0", "1-3", "4-10", "11+"], [0, 3, 10]);
export const bucketConflicts = bucketer(CONFLICTS, [0, 1, 5]);
export const bucketScale = bucketer(SCALE, [999, 9_999, 49_999]);
export const bucketBytes = bucketer(SIZE, [10_239, 102_399, 1_048_575]);
export const bucketPayload = bucketer(PAYLOAD, [1_023, 10_239, 102_399, 1_048_575]);
export const bucketOps = bucketer(OPS, [1, 9, 99, 999]);

export type ExtClass = (typeof EXT_CLASS)[number];
const EXT_CLASSES: Record<Exclude<ExtClass, "other" | "none">, readonly string[]> = {
  code: ["go", "ts", "tsx", "js", "jsx", "py", "rs", "java", "kt", "c", "h", "cc", "cpp", "rb", "sh", "swift", "cs", "php", "css", "scss"],
  markup: ["html", "htm", "xml", "svg", "vue"],
  config: ["json", "yaml", "yml", "toml", "ini", "env", "mod", "sum", "lock", "conf"],
  doc: ["md", "mdx", "txt", "rst", "adoc"],
  data: ["csv", "tsv", "sql", "log", "ndjson"],
};

/** Returns only the class; the path and extension never leave this function. */
export function extClass(path: string): ExtClass {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const dot = base.lastIndexOf(".");
  if (dot <= 0 || dot === base.length - 1) return "none";
  const ext = base.slice(dot + 1).toLowerCase();
  for (const [cls, exts] of Object.entries(EXT_CLASSES)) if (exts.includes(ext)) return cls as ExtClass;
  return "other";
}

export const searchOpts = (o: { regex: boolean; caseSensitive: boolean; wholeWord: boolean; globs: boolean; ignored: boolean; large: boolean }): SearchOpts =>
  `r${+o.regex}c${+o.caseSensitive}w${+o.wholeWord}g${+o.globs}i${+o.ignored}l${+o.large}` as SearchOpts;
