/**
 * Replay fixture schema and lint (ADR-007, ADR-008). The `device` privacy fields (`scrubbed`,
 * `signoff`) are self-attested, so the lint is a soft guard, not proof that a trace is clean.
 */
import type { MobileInputEvent } from "./types";

export type FixtureSource = "synthetic" | "device";

/** Link and session boundaries the harness applies between IME events (Story 1.2.4). */
export type BoundaryStep = { kind: "linkDown" } | { kind: "linkUp" } | { kind: "sessionSwitch" };
export type FixtureStep = MobileInputEvent | BoundaryStep;

export interface ReplayFixture {
  id: string;
  source: FixtureSource;
  keyboard: string;
  notes: string;
  events: FixtureStep[];
  /** Exact concatenation of every byte handed to the sink. */
  expectedBytes: string;
  /** Model terminal line (after the prompt) once the trace has been applied. */
  finalText: string;
  /** Anomalies the trace must raise, in order. Defaults to none. */
  expectedAnomalies?: string[];
  /** DECSET 2004 state during the trace. Defaults to off. */
  bracketedPaste?: boolean;
  /** Overrides on DEFAULT_POLICY, e.g. `backspaceStrategy`. */
  policy?: { backspaceStrategy?: "sentinel" | "keydown8"; wordDeleteStrategy?: "ctrl-w" | "del-run" };
  /** `device` only: the scrub checklist (Story 4.1.3) was completed. */
  scrubbed?: boolean;
  /** `device` only: owner sign-off text. */
  signoff?: string;
}

const EVENT_KINDS = [
  "keydown",
  "beforeinput",
  "input",
  "compositionstart",
  "compositionupdate",
  "compositionend",
  "selectionchange",
  "blur",
  "tick",
  "linkDown",
  "linkUp",
  "sessionSwitch",
] as const;

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** Structural problems with a parsed fixture; empty means it is replayable. */
export function validateFixture(raw: unknown): string[] {
  if (!isRecord(raw)) return ["fixture is not an object"];
  const errors: string[] = [];
  for (const key of ["id", "keyboard", "notes", "expectedBytes", "finalText"] as const) {
    if (typeof raw[key] !== "string") errors.push(`missing string field "${key}"`);
  }
  if (raw.source !== "synthetic" && raw.source !== "device") errors.push('field "source" must be "synthetic" or "device"');
  if (typeof raw.notes === "string" && raw.notes.trim() === "") errors.push('field "notes" must state a source URL or spike reference');
  if (!Array.isArray(raw.events) || raw.events.length === 0) {
    errors.push('"events" must be a non-empty array');
  } else {
    raw.events.forEach((e, i) => {
      if (!isRecord(e) || !(EVENT_KINDS as readonly unknown[]).includes(e.kind)) errors.push(`events[${i}] has unknown kind`);
    });
  }
  return errors;
}

const SECRET_PATTERNS: ReadonlyArray<readonly [string, RegExp]> = [
  ["GitHub token", /\bgh[pousr]_[A-Za-z0-9]{6,}/],
  ["GitHub fine-grained token", /\bgithub_pat_[A-Za-z0-9_]{10,}/],
  ["AWS access key", /\b(?:AKIA|ASIA)[0-9A-Z]{16}\b/],
  ["Slack token", /\bxox[abprs]-[A-Za-z0-9-]{8,}/],
  ["API key (sk-)", /\bsk-[A-Za-z0-9_-]{16,}/],
  ["private key block", /-----BEGIN [A-Z ]*PRIVATE KEY-----/],
  ["JWT", /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\./],
  ["bearer credential", /\bBearer\s+[A-Za-z0-9._~+/=-]{16,}/i],
  ["credential in URL", /:\/\/[^/\s:@]+:[^/\s@]+@/],
  ["password assignment", /\b(?:password|passwd|secret|token)\s*[=:]\s*\S{6,}/i],
];

/** Names of secret-shaped strings found in `text` (the raw fixture file, not just typed content). */
export function scanSecrets(text: string): string[] {
  return SECRET_PATTERNS.filter(([, re]) => re.test(text)).map(([name]) => name);
}

/** Lint problems for one committed fixture file, each prefixed with the file name. */
export function lintFixtureFile(fileName: string, rawText: string): string[] {
  const problems: string[] = [];
  const add = (msg: string) => problems.push(`${fileName}: ${msg}`);
  let parsed: unknown;
  try {
    parsed = JSON.parse(rawText);
  } catch {
    add("not valid JSON");
    return problems;
  }
  for (const e of validateFixture(parsed)) add(e);
  if (isRecord(parsed) && parsed.source === "device") {
    if (parsed.scrubbed !== true) add('source "device" requires scrubbed: true');
    if (typeof parsed.signoff !== "string" || parsed.signoff.trim() === "") add('source "device" requires a non-empty signoff');
  }
  for (const name of scanSecrets(rawText)) add(`secret-shaped string (${name})`);
  return problems;
}
