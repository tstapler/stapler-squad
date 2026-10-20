"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import {
  BacklogService,
  WorkflowEngineMode,
  WorkflowEngineReviewState,
  WorkflowEngineSoakVerdict,
  type GetWorkflowEngineStatusResponse,
} from "@/gen/session/v1/backlog_pb";
import { formatRelativeTime } from "@/lib/utils/datetime";

export type CallerClass = "operator" | "agent" | "system";
export type ClassMode = "enforce" | "shadow" | "off" | "unknown";
export type EngineModeKind = "loading" | "unknown" | "enforced" | "partial" | "shadow" | "off";

/** Display order is also the recommended enforcement order (design/ux.md section 3). */
export const CALLER_CLASSES: readonly CallerClass[] = ["operator", "agent", "system"];

const CLASS_LABEL: Record<CallerClass, string> = {
  operator: "Operators (this UI)",
  agent: "Agents",
  system: "Automation",
};
const CLASS_NOUN: Record<CallerClass, string> = {
  operator: "operators",
  agent: "agents",
  system: "automation",
};
const CLASS_FLAG_ROW: Record<CallerClass, string> = {
  operator: "flag-row-backlog-workflow-engine-enforce",
  agent: "flag-row-backlog-workflow-engine-enforce-agent",
  system: "flag-row-backlog-workflow-engine-enforce-system",
};

export interface EngineReviewView {
  state: "overdue" | "not_due" | "enforced";
  /** YYYY-MM-DD (UTC) */
  date: string;
}

export interface EngineModeView {
  kind: EngineModeKind;
  /** Per-class mode from the last good status; all "unknown" while loading or when there is none. */
  classes: Record<CallerClass, ClassMode>;
  /** Shadow differences in the last 24h (the server reports a floor past 200); null when unavailable. */
  divergence24h: number | null;
  /** True when a refetch failed after at least one good value. */
  stale: boolean;
  lastCheckedAt: number | null;
  /** The running engine fell back to built-in rules (sticky until restart). */
  fallbackActive: boolean;
  invalidatorWired: boolean;
  review: EngineReviewView | null;
  /** Feature-flag row for the first class not yet enforced (operators when all are). */
  flagRowId: string;
}

export type SoakVerdictKind = "ready" | "not_ready" | "needs_ack" | "unavailable";

/** Decision-time evidence for flipping one caller class (GetWorkflowEngineStatus.class_readiness). */
export interface ClassReadinessView {
  /** Shadow differences in the last 24h (the server reports a floor past 200). */
  divergence24h: number;
  verdict: SoakVerdictKind;
  /** Items that would lose an exit: one preview count, identical for every class. */
  strandedItemCount: number;
}

function toVerdict(v: WorkflowEngineSoakVerdict): SoakVerdictKind {
  switch (v) {
    case WorkflowEngineSoakVerdict.READY:
      return "ready";
    case WorkflowEngineSoakVerdict.NOT_READY:
      return "not_ready";
    case WorkflowEngineSoakVerdict.NEEDS_OWNER_ACK:
      return "needs_ack";
    default:
      return "unavailable";
  }
}

/** Readiness for one class, or null when the server did not report it (callers fail closed). */
export function readinessFromResponse(
  resp: GetWorkflowEngineStatusResponse,
  cls: CallerClass
): ClassReadinessView | null {
  const r = resp.classReadiness?.[cls];
  if (!r) return null;
  return {
    divergence24h: Number(r.divergence24h),
    verdict: toVerdict(r.soakVerdict),
    strandedItemCount: Number(r.strandedItemCount),
  };
}

const UNKNOWN_CLASSES: Record<CallerClass, ClassMode> = { operator: "unknown", agent: "unknown", system: "unknown" };

function toClassMode(m: WorkflowEngineMode | undefined): ClassMode {
  switch (m) {
    case WorkflowEngineMode.ENFORCE:
      return "enforce";
    case WorkflowEngineMode.SHADOW:
      return "shadow";
    case WorkflowEngineMode.OFF:
      return "off";
    default:
      return "unknown";
  }
}

function classifyKind(classes: Record<CallerClass, ClassMode>): EngineModeKind {
  const modes = CALLER_CLASSES.map((c) => classes[c]);
  // A class the server did not report is unknowable: fail closed rather than guess.
  if (modes.includes("unknown")) return "unknown";
  const enforced = modes.filter((m) => m === "enforce").length;
  if (enforced === modes.length) return "enforced";
  if (enforced > 0) return "partial";
  return modes.includes("shadow") ? "shadow" : "off";
}

function reviewFrom(resp: GetWorkflowEngineStatusResponse): EngineReviewView | null {
  if (!resp.reviewDueAt) return null;
  const date = new Date(Number(resp.reviewDueAt.seconds) * 1000).toISOString().slice(0, 10);
  switch (resp.reviewState) {
    case WorkflowEngineReviewState.OVERDUE:
      return { state: "overdue", date };
    case WorkflowEngineReviewState.NOT_DUE:
      return { state: "not_due", date };
    case WorkflowEngineReviewState.ENFORCED:
      return { state: "enforced", date };
    default:
      return null;
  }
}

function viewFromStatus(resp: GetWorkflowEngineStatusResponse, stale: boolean, lastCheckedAt: number): EngineModeView {
  const classes = {
    operator: toClassMode(resp.classModes?.operator),
    agent: toClassMode(resp.classModes?.agent),
    system: toClassMode(resp.classModes?.system),
  };
  const firstUnenforced = CALLER_CLASSES.find((c) => classes[c] !== "enforce") ?? "operator";
  return {
    // An engine whose gate is not wired enforces nothing whatever the flags say.
    kind: resp.gateWired ? classifyKind(classes) : "unknown",
    classes,
    divergence24h: Number(resp.divergence24h),
    stale,
    lastCheckedAt,
    fallbackActive: resp.fallbackActive,
    invalidatorWired: resp.invalidatorWired,
    review: reviewFrom(resp),
    flagRowId: CLASS_FLAG_ROW[firstUnenforced],
  };
}

function emptyView(kind: "loading" | "unknown"): EngineModeView {
  return {
    kind,
    classes: UNKNOWN_CLASSES,
    divergence24h: null,
    stale: false,
    lastCheckedAt: null,
    fallbackActive: false,
    invalidatorWired: true,
    review: null,
    flagRowId: CLASS_FLAG_ROW.operator,
  };
}

// ---------------------------------------------------------------------------
// Copy (design/ux.md section 2). One source so the banner, the stage-form
// reminder and the save confirmation cannot disagree.
// ---------------------------------------------------------------------------

function classModePhrase(m: ClassMode): string {
  switch (m) {
    case "enforce":
      return "enforced";
    case "shadow":
      return "shadow, not enforced";
    case "off":
      return "off, not enforced";
    default:
      return "unknown, not confirmed enforced";
  }
}

function shadowDifferences(n: number | null): string {
  if (n === null) return "Difference count unavailable.";
  if (n === 0) return "No differences in the last 24h.";
  return `${n} ${n === 1 ? "difference" : "differences"} in the last 24h.`;
}

export const FALLBACK_NOTE = "The engine fell back to the built-in rules after an error, so edits may not be applied.";

function fallbackAffectsClaim(v: EngineModeView): boolean {
  return v.fallbackActive && (v.kind === "enforced" || v.kind === "partial");
}

/** Mode sentence shown in the banner (without the Stale suffix). */
export function bannerSentence(v: EngineModeView): string {
  switch (v.kind) {
    case "loading":
      return "Checking whether these settings are enforced...";
    case "unknown":
      return "Couldn't determine whether these settings are enforced. Treat edits as not applied.";
    case "enforced":
      return "Enforced: edits here change which transitions and gates the backlog allows, for operators, agents and automation. Edges that automation relies on are fixed; their gates are editable.";
    case "partial":
      return `Partly enforced. ${CALLER_CLASSES.map((c) => `${CLASS_LABEL[c]}: ${classModePhrase(v.classes[c])}.`).join(" ")}`;
    case "shadow":
      return `Shadow: edits are compared against the built-in rules but are not enforced. ${shadowDifferences(v.divergence24h)}`;
    case "off":
      return "Not in use: the built-in 9-stage workflow is active. Edits are saved but have no effect.";
  }
}

function lastCheckedAgo(v: EngineModeView): string | null {
  if (!v.stale || v.lastCheckedAt === null) return null;
  return formatRelativeTime(v.lastCheckedAt).toLowerCase();
}

/** Everything the banner prints and announces as its mode text: sentence, Stale suffix and fallback note. */
export function bannerText(v: EngineModeView): string {
  const ago = lastCheckedAgo(v);
  return [
    bannerSentence(v),
    ago && `Last checked ${ago}; couldn't refresh.`,
    fallbackAffectsClaim(v) && FALLBACK_NOTE,
  ]
    .filter(Boolean)
    .join(" ");
}

const KIND_SHORT: Partial<Record<EngineModeKind, string>> = {
  enforced: "Enforced",
  partial: "Partly enforced",
  shadow: "Shadow",
  off: "Off",
};

function joinNames(names: string[]): string {
  if (names.length <= 1) return names.join("");
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

function partialSummary(v: EngineModeView): string {
  const enforced = CALLER_CLASSES.filter((c) => v.classes[c] === "enforce").map((c) => CLASS_NOUN[c]);
  const groups = new Map<ClassMode, CallerClass[]>();
  for (const c of CALLER_CLASSES) {
    if (v.classes[c] !== "enforce") groups.set(v.classes[c], [...(groups.get(v.classes[c]) ?? []), c]);
  }
  const clauses = [...groups.entries()].map(([mode, cs]) => {
    const verb = cs.length === 1 && cs[0] === "system" ? "is" : "are";
    return `${joinNames(cs.map((c) => CLASS_NOUN[c]))} ${verb} ${mode === "off" ? "off" : "in shadow"}`;
  });
  return `Saved edits apply to ${joinNames(enforced)} only; ${clauses.join("; ")}.`;
}

/** One-line reminder directly above the stage form's Save button (UX-41, UX-52). Never empty. */
export function formReminder(v: EngineModeView): string {
  const ago = lastCheckedAgo(v);
  if (ago) {
    return `Mode: ${KIND_SHORT[v.kind] ?? "unknown"} (last checked ${ago}; couldn't refresh). Saved edits may not be enforced.`;
  }
  switch (v.kind) {
    case "loading":
      return "Mode: checking...";
    case "unknown":
      return "Mode: couldn't be determined. Treat saved edits as not applied.";
    case "enforced":
      return fallbackAffectsClaim(v) ? `Mode: Enforced. ${FALLBACK_NOTE}` : "Mode: Enforced.";
    case "partial":
      return `Mode: Partly enforced. ${partialSummary(v)}${fallbackAffectsClaim(v) ? ` ${FALLBACK_NOTE}` : ""}`;
    case "shadow":
      return "Mode: Shadow. Saved edits are not enforced yet.";
    case "off":
      return "Mode: Off. Edits are saved but have no effect.";
  }
}

/** Confirmation after a successful stage save; never claims enforcement for a class that is not enforced. */
export function saveConfirmation(v: EngineModeView): string {
  let text: string;
  if (v.stale || v.kind === "loading" || v.kind === "unknown") {
    text = "Saved. Couldn't confirm whether it is enforced.";
  } else if (v.kind === "enforced") {
    text = "Saved and enforced";
  } else if (v.kind === "partial") {
    const enforced = CALLER_CLASSES.filter((c) => v.classes[c] === "enforce").map((c) => CLASS_NOUN[c]);
    const rest = CALLER_CLASSES.filter((c) => v.classes[c] !== "enforce").map((c) => CLASS_NOUN[c]);
    text = `Saved. Enforced for ${joinNames(enforced)} only; not enforced for ${joinNames(rest)}.`;
  } else if (v.kind === "shadow") {
    text = "Saved. Not enforced yet (shadow)";
  } else {
    text = "Saved. Not in use (off)";
  }
  const extras: string[] = [];
  if (!v.invalidatorWired) extras.push("Saved, but the running engine was not refreshed.");
  if (fallbackAffectsClaim(v)) extras.push(FALLBACK_NOTE);
  return [text, ...extras].join(" ");
}

export function reviewLine(r: EngineReviewView): string {
  switch (r.state) {
    case "overdue":
      return `Review overdue: the soak review date (${r.date}) has passed and no caller class is enforced. Review the soak evidence in docs/how-to/flip-workflow-engine-enforcement.md.`;
    case "not_due":
      return `Soak review due ${r.date}.`;
    case "enforced":
      return `Soak review date ${r.date}.`;
  }
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/** Focus/visibility refetches are coalesced to at most one per this window. */
export const ENGINE_STATUS_REFETCH_WINDOW_MS = 5000;

export interface UseEngineModeStatusReturn {
  view: EngineModeView;
  /** Re-fetch now (Retry, or after a successful stage save). */
  refetch: () => Promise<void>;
}

/**
 * One GetWorkflowEngineStatus fetch shared by the Stages banner and StageForm
 * (the page calls this once and passes the view down). Fetches on mount and
 * again when the tab regains focus or becomes visible; there is no polling
 * timer. A failed refetch after a good value keeps that value and marks it
 * stale; it never promotes the text.
 */
export function useEngineModeStatus(): UseEngineModeStatusReturn {
  const [last, setLast] = useState<{ resp: GetWorkflowEngineStatusResponse; at: number } | null>(null);
  const [failed, setFailed] = useState(false);
  const [settled, setSettled] = useState(false);
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);
  const lastStartRef = useRef(0);
  const mountedRef = useRef(true);

  const refetch = useCallback(async () => {
    if (!clientRef.current) clientRef.current = createClient(BacklogService, getConnectTransport());
    lastStartRef.current = Date.now();
    try {
      const resp = await clientRef.current.getWorkflowEngineStatus({});
      if (!mountedRef.current) return;
      setLast({ resp, at: Date.now() });
      setFailed(false);
    } catch (err) {
      console.error("[useEngineModeStatus] getWorkflowEngineStatus:", err);
      if (mountedRef.current) setFailed(true);
    } finally {
      if (mountedRef.current) setSettled(true);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void refetch();
    const onWake = () => {
      if (document.visibilityState === "hidden") return;
      if (Date.now() - lastStartRef.current < ENGINE_STATUS_REFETCH_WINDOW_MS) return;
      void refetch();
    };
    document.addEventListener("visibilitychange", onWake);
    window.addEventListener("focus", onWake);
    return () => {
      mountedRef.current = false;
      document.removeEventListener("visibilitychange", onWake);
      window.removeEventListener("focus", onWake);
    };
  }, [refetch]);

  const view = useMemo<EngineModeView>(() => {
    if (last) return viewFromStatus(last.resp, failed, last.at);
    return emptyView(settled ? "unknown" : "loading");
  }, [last, failed, settled]);

  return { view, refetch };
}
