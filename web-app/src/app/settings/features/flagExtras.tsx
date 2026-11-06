import type { ComponentType, ReactNode } from "react";
import type { FeatureFlagMeta, FlagChange, SetFlagResult } from "@/lib/contexts/FeatureFlagsContext";
import { GLOBAL_SCOPE } from "@/lib/contexts/featureFlagScopes";
import { PiDisableWarningDialog } from "@/components/settings/PiDisableWarningDialog";
import { PI_SUPPORT_FLAG_NAME } from "@/lib/constants/programs";
import { GateStatusLine } from "./GateStatusLine";
import { GateKindOverrides, type OverrideMode } from "./GateKindOverrides";
import { Link } from "@/lib/nav";
import { routes } from "@/lib/routes";
import {
  CALLER_CLASSES,
  FALLBACK_NOTE,
  lastCheckedAgo,
  reviewLine,
  type CallerClass,
  type ClassMode,
  type EngineModeView,
} from "@/lib/hooks/useEngineModeStatus";
import { engineReviewOverdue, engineIntro, engineSummary, flagDescription, rowControlButton, rowControls } from "./page.css";

// Mirrors config.HiddenSessionGateFeatureFlag on the server.
export const HIDDEN_SESSION_GATE_FLAG = "hidden_session_gate";
const DIAGNOSE_NUDGE_FLAG = "diagnose_nudge_enabled";
const STALL_NUDGE_FLAG = "autonomous_stall_nudge_enabled";

// The notification flags are read live by the page (FG-1): no reload after a flip.
const LIVE_FLAG_NOTE = "Takes effect without reload";

export const FEATURE_META: Record<string, { label: string }> = {
  backlog: { label: "Backlog" },
  "programs:cli-flag-probe": { label: "Program flag discovery (check binary and read --help)" },
  "pi-support": { label: "pi coding agent" },
  "backlog:sdd-default-pipeline": { label: "Backlog: default new items to SDD pipeline" },
  "backlog-triage-results-panel": { label: "Backlog: formatted triage results panel" },
  "backlog-review-results-panel": { label: "Backlog: formatted review results panel" },
  "review:block-approval-on-ci-failure": { label: "Block Approve when CI is failing" },
  "terminal:resync-visibility-scope": { label: "Terminal resync: scope to visible terminal" },
  "terminal:resync-correlation-id": { label: "Terminal resync: correlation IDs" },
  "terminal:resync-skip-stale-dimension-slowpath": {
    label: "Terminal resync: skip slow path for backgrounded terminals",
  },
  "terminal:resync-exec-gate-fast-lane": { label: "Terminal resync: exec-gate fast lane" },
  "terminal:resync-stagger": { label: "Terminal resync: stagger bursts" },
  "terminal:resync-compression": { label: "Terminal resync: wire compression" },
  "terminal:resync-batching": { label: "Terminal resync: batch requests" },
  [HIDDEN_SESSION_GATE_FLAG]: { label: "Notifications: hidden-session delivery gate" },
  [DIAGNOSE_NUDGE_FLAG]: { label: "Diagnose & Nudge" },
  [STALL_NUDGE_FLAG]: { label: "Autonomous stall nudge" },
  "backlog:workflow-engine-shadow": { label: "Workflow engine: shadow (compare only)" },
  "backlog:workflow-engine-enforce": { label: "Enforce for operators (this UI)" },
  "backlog:workflow-engine-enforce-agent": { label: "Enforce for agents (MCP tools)" },
  "backlog:workflow-engine-enforce-system": { label: "Enforce for automation (lifecycle, sync, pollers)" },
};

export type FlagSetter = (name: string, change: boolean | FlagChange) => Promise<SetFlagResult | void>;

export interface FlagExtraContext {
  flag: FeatureFlagMeta;
  /** True while a change to this flag is in flight. */
  pending: boolean;
  setFlag: FlagSetter;
  /** Turns the flag Off through the row's own save path (one in-flight change per flag, no dialog). */
  revert?: () => void;
}

/** What makes a flag one caller class's enforce switch. */
export interface EnforceExtra {
  cls: CallerClass;
  stepLine: string;
  /** Extra sentence in the confirm dialog's body. */
  classNote?: string;
  /** The recommended predecessor, for the non-blocking order advisory. */
  predecessor?: { flag: string; advisory: string };
}

/**
 * Per-flag behavior the generic row renders without name-branching. Later stories
 * (the workflow-engine rows, the quota link) add entries here instead of editing
 * the page.
 */
export interface FlagExtra {
  /** Section heading the flag is listed under, in first-seen order. */
  group?: string;
  /** Render this flag nested under the named flag. */
  parent?: string;
  /** Shown when the server reports the flag as On but inactive. */
  inertNote?: string;
  /** Replaces the default (no) live note. */
  liveNote?: string;
  /** Builds the change sent for a toggle to `next`; default is the legacy boolean. */
  toggleChange?: (next: boolean) => boolean | FlagChange;
  /** Turning the flag off is first checked and, when `needsConfirm` resolves true, confirmed in `Dialog`. */
  confirmDisable?: {
    needsConfirm: () => Promise<boolean>;
    Dialog: ComponentType<{ onAcknowledge: () => void; onCancel: () => void }>;
  };
  renderExtras?: (ctx: FlagExtraContext) => ReactNode;
  /** Turning the flag On opens the readiness-evidence confirm (workflow-engine enforce rows). */
  enforce?: EnforceExtra;
}

// Fail closed: a failed or non-2xx status check shows the warning rather than
// silently disabling (ADR-003). The check runs server-side because ~/.pi is on
// the server's filesystem, not the browser's.
async function piExtensionInstalled(): Promise<boolean> {
  try {
    const res = await fetch("/api/pi-extension-status");
    if (!res.ok) return true;
    const data: { installed?: boolean } = await res.json();
    return Boolean(data.installed);
  } catch {
    return true;
  }
}

function changeKindOverride(setFlag: FlagSetter, scope: string, mode: OverrideMode) {
  void setFlag(
    HIDDEN_SESSION_GATE_FLAG,
    mode === "inherit" ? { mutation: "clear", scope } : { mutation: "set", scope, enabled: mode === "on" },
  );
}

const AUTONOMOUS_GROUP = "Autonomous backlog";

// ---------------------------------------------------------------------------
// Workflow engine (Story 1.3.3). The mode reader caches about 1s, so these
// flags take effect "within a few seconds", not instantly.
// ---------------------------------------------------------------------------

const ENGINE_GROUP = "Workflow engine";
const ENGINE_LIVE_NOTE = "Takes effect within a few seconds, without reload";
export const ENGINE_SHADOW_FLAG = "backlog:workflow-engine-shadow";
// Mirrors config.WorkflowEngineEnforce{,Agent,System}FeatureFlag on the server.
export const ENGINE_ENFORCE_FLAGS: Record<CallerClass, string> = {
  operator: "backlog:workflow-engine-enforce",
  agent: "backlog:workflow-engine-enforce-agent",
  system: "backlog:workflow-engine-enforce-system",
};
const CLASS_NOUN: Record<CallerClass, string> = { operator: "operators", agent: "agents", system: "automation" };

/** DOM id of a flag's row; the Stages banner links to these (the id contract). */
export function flagRowId(flagName: string): string {
  return `flag-row-${flagName.replace(/:/g, "-")}`;
}

/** True for flags whose On state is one caller class's enforcement. */
export function isEngineEnforceFlag(flagName: string): boolean {
  return Object.values(ENGINE_ENFORCE_FLAGS).includes(flagName);
}

/** True for the shadow flag and the enforce flags: a change to any of them changes the engine status. */
export function isEngineModeFlag(flagName: string): boolean {
  return flagName === ENGINE_SHADOW_FLAG || isEngineEnforceFlag(flagName);
}

/** Per-class mode: "enforce" only when the flag is on AND the engine reports enforce; a flag the engine has not picked up yet falls back to the reported mode. */
export function classModesFromFlags(view: EngineModeView, flagList: FeatureFlagMeta[]): Record<CallerClass, ClassMode> {
  const shadowOn = flagList.find((f) => f.name === ENGINE_SHADOW_FLAG)?.enabled ?? false;
  const modes = {} as Record<CallerClass, ClassMode>;
  for (const cls of CALLER_CLASSES) {
    const flag = flagList.find((f) => f.name === ENGINE_ENFORCE_FLAGS[cls]);
    const reported = view.classes[cls];
    if (!flag) modes[cls] = "unknown";
    else if (flag.enabled && reported === "enforce") modes[cls] = "enforce";
    else modes[cls] = reported === "shadow" || reported === "off" ? reported : shadowOn ? "shadow" : "off";
  }
  return modes;
}

function namesOf(classes: CallerClass[]): string {
  const names = classes.map((c) => CLASS_NOUN[c]);
  return names.length <= 1 ? names.join("") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

function partialClauses(modes: Record<CallerClass, ClassMode>): string {
  const rest = CALLER_CLASSES.filter((c) => modes[c] !== "enforce");
  const byMode = new Map<ClassMode, CallerClass[]>();
  for (const c of rest) byMode.set(modes[c], [...(byMode.get(modes[c]) ?? []), c]);
  return [...byMode.entries()].map(([m, cs]) => `${namesOf(cs)} still ${m === "off" ? "off" : "in shadow"}`).join("; ");
}

function unconfirmedClasses(modes: Record<CallerClass, ClassMode>, flagList: FeatureFlagMeta[]): CallerClass[] {
  return CALLER_CLASSES.filter((c) => modes[c] !== "enforce" && flagList.find((f) => f.name === ENGINE_ENFORCE_FLAGS[c])?.enabled);
}

/** Count and plain-words state of the three enforce flags; the wording matches the Stages banner (UX-39). */
export function workflowEngineSummary(view: EngineModeView, flagList: FeatureFlagMeta[]): string {
  const base = engineSummaryBase(view, flagList);
  if (view.kind === "loading") return base;
  const modes = classModesFromFlags(view, flagList);
  // "Mode unavailable" already says nothing is confirmed.
  const unavailable = view.kind === "unknown" || CALLER_CLASSES.some((c) => modes[c] === "unknown");
  const unconfirmed = unavailable ? [] : unconfirmedClasses(modes, flagList);
  const ago = lastCheckedAgo(view);
  return [
    base,
    unconfirmed.length > 0 && `On, not yet confirmed by the engine: ${namesOf(unconfirmed)}.`,
    ago && `Last checked ${ago}; couldn't refresh.`,
  ]
    .filter(Boolean)
    .join(" ");
}

function engineSummaryBase(view: EngineModeView, flagList: FeatureFlagMeta[]): string {
  if (view.kind === "loading") return "Checking the engine status...";
  const modes = classModesFromFlags(view, flagList);
  if (view.kind === "unknown" || CALLER_CLASSES.some((c) => modes[c] === "unknown")) {
    return "Mode unavailable: couldn't read the engine status.";
  }
  const enforced = CALLER_CLASSES.filter((c) => modes[c] === "enforce");
  const fallback = view.fallbackActive && enforced.length > 0 ? ` ${FALLBACK_NOTE}` : "";
  if (enforced.length === CALLER_CLASSES.length) return `All 3 caller classes enforced.${fallback}`;
  if (enforced.length === 0) {
    const anyShadow = CALLER_CLASSES.some((c) => modes[c] === "shadow");
    return anyShadow
      ? "0 of 3 caller classes enforced. Shadow: edits are compared with the built-in rules but not applied."
      : "0 of 3 caller classes enforced. Off: the built-in rules apply and stage edits have no effect.";
  }
  return `${enforced.length} of 3 caller classes enforced (Partly enforced): ${namesOf(enforced)} enforced; ${partialClauses(modes)}.${fallback}`;
}

/** Number of enforce flags that will be On once `changed` is set to `next`. */
export function enforcedCountAfter(flagList: FeatureFlagMeta[], changed: string, next: boolean): number {
  return Object.values(ENGINE_ENFORCE_FLAGS).filter((name) =>
    name === changed ? next : flagList.find((f) => f.name === name)?.enabled,
  ).length;
}

export interface GroupHeaderProps {
  view: EngineModeView;
  flagList: FeatureFlagMeta[];
}

function WorkflowEngineGroupHeader({ view, flagList }: GroupHeaderProps) {
  const review = view.review;
  return (
    <>
      <p className={engineIntro}>
        These three switches are independent. Recommended order: operators, then agents, then automation.
      </p>
      <p className={engineSummary} data-testid="workflow-engine-summary">
        {workflowEngineSummary(view, flagList)}
      </p>
      {review && (
        <p
          className={review.state === "overdue" ? engineReviewOverdue : flagDescription}
          data-testid="workflow-engine-review"
        >
          {reviewLine(review)}
        </p>
      )}
    </>
  );
}

/** Content rendered under a group's heading, keyed by heading. */
export const GROUP_HEADERS: Record<string, ComponentType<GroupHeaderProps>> = {
  [ENGINE_GROUP]: WorkflowEngineGroupHeader,
};

function enforceExtra(enforce: EnforceExtra): FlagExtra {
  return {
    group: ENGINE_GROUP,
    liveNote: ENGINE_LIVE_NOTE,
    enforce,
    renderExtras: ({ flag, pending, revert }) => (
      <>
        <div className={flagDescription} data-testid="feature-flag-step">
          {enforce.stepLine}
        </div>
        {flag.enabled && (
          <div className={rowControls}>
            {// analytics-exempt
            <button
              type="button"
              className={rowControlButton}
              aria-disabled={pending || undefined}
              onClick={() => {
                if (!pending) revert?.();
              }}
              data-testid="feature-flag-revert"
            >
              Revert to Off
            </button>}
            <Link href={routes.settingsBacklogStages} className={rowControlButton} data-testid="feature-flag-edit-stages">
              Edit stages
            </Link>
          </div>
        )}
      </>
    ),
  };
}

export const FLAG_EXTRAS: Record<string, FlagExtra> = {
  backlog: {
    renderExtras: () => (
      <div className={flagDescription}>
        <Link href={routes.settingsBacklogQuota} data-testid="feature-flag-quota-link">
          Backlog Quota settings
        </Link>
      </div>
    ),
  },
  [PI_SUPPORT_FLAG_NAME]: {
    confirmDisable: { needsConfirm: piExtensionInstalled, Dialog: PiDisableWarningDialog },
  },
  [DIAGNOSE_NUDGE_FLAG]: { group: AUTONOMOUS_GROUP },
  [STALL_NUDGE_FLAG]: {
    group: AUTONOMOUS_GROUP,
    parent: DIAGNOSE_NUDGE_FLAG,
    inertNote: "Has no effect until Diagnose & Nudge is On.",
  },
  [HIDDEN_SESSION_GATE_FLAG]: {
    liveNote: LIVE_FLAG_NOTE,
    // The scopable gate flag always sends the explicit "global" scope, so a
    // dropped scope can never write the global value by accident.
    toggleChange: (next) => ({ mutation: "set", scope: GLOBAL_SCOPE, enabled: next }),
    renderExtras: ({ flag, setFlag }) => (
      <>
        <GateStatusLine className={flagDescription} />
        <GateKindOverrides
          scopes={flag.scopes}
          globalEnabled={flag.enabled}
          onChange={(scope, mode) => changeKindOverride(setFlag, scope, mode)}
          onReset={() => void setFlag(HIDDEN_SESSION_GATE_FLAG, { mutation: "reset" })}
        />
      </>
    ),
  },
  [ENGINE_SHADOW_FLAG]: { group: ENGINE_GROUP, liveNote: ENGINE_LIVE_NOTE },
  [ENGINE_ENFORCE_FLAGS.operator]: enforceExtra({
    cls: "operator",
    stepLine: "Step 1 of 3 (recommended first).",
  }),
  [ENGINE_ENFORCE_FLAGS.agent]: enforceExtra({
    cls: "agent",
    stepLine: "Step 2 of 3. Enforce after operators have soaked.",
    predecessor: {
      flag: ENGINE_ENFORCE_FLAGS.operator,
      advisory: "Operators are not enforced yet; the recommended order is operators first.",
    },
  }),
  [ENGINE_ENFORCE_FLAGS.system]: enforceExtra({
    cls: "system",
    stepLine: "Step 3 of 3. Enforce last; it needs the denied-transition recovery in the deployed binary.",
    classNote:
      "Blocked automated transitions appear as stuck items (transition_denied) until the gate is satisfied.",
    predecessor: {
      flag: ENGINE_ENFORCE_FLAGS.agent,
      advisory: "Agents are not enforced yet; the recommended order is agents before automation.",
    },
  }),
};

/** One entry of the rendered list: a standalone flag, or a headed group of flags. */
export type FlagLayoutItem =
  | { kind: "flag"; flag: FeatureFlagMeta; children: FeatureFlagMeta[] }
  | { kind: "group"; heading: string; members: Array<{ flag: FeatureFlagMeta; children: FeatureFlagMeta[] }> };

/**
 * Orders the server's list for display: nested flags follow their parent, and
 * grouped flags are collected under one heading at the position of the group's
 * first member. Flags without an extra keep the server's order.
 */
export function layoutFlags(flagList: FeatureFlagMeta[]): FlagLayoutItem[] {
  const present = new Set(flagList.map((f) => f.name));
  const nestedUnder = (f: FeatureFlagMeta) => {
    const parent = FLAG_EXTRAS[f.name]?.parent;
    return parent && present.has(parent) ? parent : undefined;
  };
  const childrenOf = (name: string) => flagList.filter((c) => nestedUnder(c) === name);

  const items: FlagLayoutItem[] = [];
  const groups = new Map<string, Extract<FlagLayoutItem, { kind: "group" }>>();
  for (const flag of flagList) {
    if (nestedUnder(flag)) continue;
    const entry = { flag, children: childrenOf(flag.name) };
    const heading = FLAG_EXTRAS[flag.name]?.group;
    if (!heading) {
      items.push({ kind: "flag", ...entry });
      continue;
    }
    let group = groups.get(heading);
    if (!group) {
      group = { kind: "group", heading, members: [] };
      groups.set(heading, group);
      items.push(group);
    }
    group.members.push(entry);
  }
  return items;
}
