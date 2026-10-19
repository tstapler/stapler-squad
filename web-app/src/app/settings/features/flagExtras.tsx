import type { ComponentType, ReactNode } from "react";
import type { FeatureFlagMeta, FlagChange, SetFlagResult } from "@/lib/contexts/FeatureFlagsContext";
import { GLOBAL_SCOPE } from "@/lib/contexts/featureFlagScopes";
import { PiDisableWarningDialog } from "@/components/settings/PiDisableWarningDialog";
import { PI_SUPPORT_FLAG_NAME } from "@/lib/constants/programs";
import { GateStatusLine } from "./GateStatusLine";
import { GateKindOverrides, type OverrideMode } from "./GateKindOverrides";
import Link from "next/link";
import { routes } from "@/lib/routes";
import { flagDescription } from "./page.css";

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
};

export type FlagSetter = (name: string, change: boolean | FlagChange) => Promise<SetFlagResult | void>;

export interface FlagExtraContext {
  flag: FeatureFlagMeta;
  /** True while a change to this flag is in flight. */
  pending: boolean;
  setFlag: FlagSetter;
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
