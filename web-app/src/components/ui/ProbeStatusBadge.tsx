// +feature: settings-programs
"use client";

import { useEffect, useState } from "react";
import type { ProbeUiState } from "@/lib/hooks/useProbeProgram";
import * as css from "./ProbeStatusBadge.css";

export const CHECKING_ANNOUNCE_DELAY_MS = 300;

export const NOT_FOUND_TEXT =
  "Not found as an executable on the server's PATH. Shell aliases and functions are not checked; the program may still work when launched from your shell.";
export const COULDNT_CHECK_TEXT = "Couldn't check right now.";
const TRANSPORT_HINT =
  "The server may refuse checks when it listens on a non-loopback address without authentication.";

type Tone = "success" | "neutral" | "warning";
type IconName = "check" | "info" | "warning";

interface Row {
  tone: Tone;
  icons: IconName[];
  text: string;
  extra?: string;
  path?: string;
  retry?: boolean;
  flags?: string;
}

function describe(state: ProbeUiState, token: string): Row | null {
  switch (state.kind) {
    case "checking":
      return { tone: "neutral", icons: [], text: "Checking..." };
    case "found":
      return {
        tone: "success",
        icons: ["check"],
        text: `Found: ${state.path}`,
        path: state.path,
        flags: state.flagCount > 0 ? `${state.flagCount} flags detected` : undefined,
      };
    case "noFlags":
      return {
        tone: "neutral",
        icons: ["check"],
        text: `Found: ${state.path}. Couldn't read flags from --help; flag suggestions unavailable.`,
        path: state.path,
      };
    case "needsConfirm":
      return {
        tone: "neutral",
        icons: ["check", "info"],
        text: `Found: ${state.path}. Not checked for flags yet. Check runs \`${token} --help\` on this server.`,
        path: state.path,
      };
    case "timeout":
      return {
        tone: "neutral",
        icons: ["check", "info"],
        text: `Found: ${state.path}. Timed out reading flags — try Check again.`,
        path: state.path,
      };
    case "wrapper":
      return {
        tone: "neutral",
        icons: ["check", "info"],
        text: `Wrapper command (${token}): flags for the wrapped program are not checked.`,
      };
    case "notFound":
      return { tone: "warning", icons: ["warning"], text: NOT_FOUND_TEXT };
    case "busyOrError":
      return { tone: "neutral", icons: ["info"], text: COULDNT_CHECK_TEXT, retry: true };
    case "transportError":
      return {
        tone: "warning",
        icons: ["warning"],
        text: COULDNT_CHECK_TEXT,
        extra: TRANSPORT_HINT,
        retry: true,
      };
    default:
      return null;
  }
}

const ICON_PATHS: Record<IconName, string> = {
  check: "M3 8.5l3.2 3.2L13 4.8",
  info: "M8 7v5M8 4.2v.1",
  warning: "M8 2.5L14 13H2L8 2.5zM8 7v3M8 11.6v.1",
};

function Icon({ name }: { name: IconName }) {
  return (
    <svg
      className={`${css.icon}`}
      data-icon={name}
      aria-hidden="true"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d={ICON_PATHS[name]} />
    </svg>
  );
}

function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;
}

export interface ProbeStatusBadgeProps {
  state: ProbeUiState;
  /** First token of the probed command ("Checked: <token>"). */
  checkedToken: string;
  onRetry: () => void;
  /** The explicit Check button; also relabelled "Check again" once shown. */
  onConfirm: () => void;
  testId?: string;
  /** Stable id for the command input's aria-describedby. */
  id?: string;
}

export function ProbeStatusBadge({ state, checkedToken, onRetry, onConfirm, testId, id }: ProbeStatusBadgeProps) {
  const checking = state.kind === "checking";
  const [announce, setAnnounce] = useState(false);
  const [checkShown, setCheckShown] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!checking) {
      setAnnounce(false);
      return;
    }
    const t = setTimeout(() => setAnnounce(true), CHECKING_ANNOUNCE_DELAY_MS);
    return () => clearTimeout(t);
  }, [checking]);

  useEffect(() => {
    if (state.kind === "needsConfirm") setCheckShown(true);
    if (state.kind === "idle" || state.kind === "disabled") {
      setCheckShown(false);
      setExpanded(false);
    }
  }, [state.kind]);

  const row = describe(state, checkedToken);
  if (!row) return null;

  const showCheck = checkShown || state.kind === "needsConfirm";
  const reduced = prefersReducedMotion();
  const isFound = row.tone === "success" || Boolean(row.path);

  return (
    <div className={`${css.root} ${css.tone[row.tone]}`} data-testid={testId} data-tone={row.tone}>
      <div className={`${css.status}`} role="status" aria-live="polite" id={id}>
        {checking ? <span className={`${reduced ? css.spinnerStatic : css.spinner}`} data-icon="spinner" data-static={reduced} aria-hidden="true" /> : null}
        {row.icons.map((name) => (
          <Icon key={name} name={name} />
        ))}
        {checking && !announce ? (
          <span aria-hidden="true">{row.text}</span>
        ) : (
          <span className={row.path && !expanded && row.tone === "success" ? `${css.path}` : undefined}>{row.text}</span>
        )}
        {row.flags ? <span>{row.flags}</span> : null}
        {row.extra ? <span>{row.extra}</span> : null}
      </div>
      {isFound && (
        <div className={`${css.detail}`}>
          Checked on this server only · Checked: {checkedToken}
          {expanded && row.path ? <code className={`${css.pathExpanded}`}> {row.path}</code> : null}
        </div>
      )}
      {row.path && (
        // analytics-exempt
        <button type="button" className={`${css.toggle}`} aria-expanded={expanded} onClick={() => setExpanded((v) => !v)}>
          {expanded ? "Hide full path" : "Show full path"}
        </button>
      )}
      {row.retry && (
        // analytics-exempt
        <button type="button" className={`${css.button}`} onClick={onRetry}>
          Retry
        </button>
      )}
      {showCheck && (
        // analytics-exempt
        <button type="button" className={`${css.button}`} onClick={onConfirm} disabled={checking} aria-busy={checking}>
          {state.kind === "needsConfirm" ? "Check" : "Check again"}
        </button>
      )}
    </div>
  );
}
