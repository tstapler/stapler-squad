/** Replays a fixture trace through the pure reducer into a model terminal. No DOM, no real clock. */
import { abandonRegion, demoteToDeferred } from "./composition";
import type { FixtureStep, ReplayFixture } from "./fixtureSchema";
import { NO_MODIFIERS, reduceWithKeymap } from "./keymap";
import { DEFAULT_POLICY, resetValue, type MobileInputPolicy } from "./policy";
import { graphemes } from "./rewrite";
import type { MobileInputEvent, MobileInputState } from "./types";

/** Terminal line as a hardware keyboard would leave it: DEL erases a grapheme, ^W a word, text appends. */
export class ModelTerminal {
  text = "";
  /** DELs that hit an empty line, i.e. would have eaten the prompt. */
  overDeletes = 0;

  apply(bytes: string): void {
    let rest = bytes.replace(/\x1b\[20[01]~/g, "");
    while (rest) {
      const cut = rest.search(/[\x7f\x17]/);
      if (cut < 0) {
        this.text += rest;
        return;
      }
      this.text += rest.slice(0, cut);
      this.erase(rest[cut] === "\x7f" ? "grapheme" : "word");
      rest = rest.slice(cut + 1);
    }
  }

  private erase(unit: "grapheme" | "word"): void {
    if (this.text === "") {
      this.overDeletes++;
      return;
    }
    this.text =
      unit === "grapheme" ? graphemes(this.text).slice(0, -1).join("") : this.text.replace(/\S+\s*$/u, "");
  }
}

export interface ReplayResult {
  id: string;
  source: ReplayFixture["source"];
  bytes: string;
  finalText: string;
  anomalies: string[];
  /** Bytes the sink refused while the link was down; never applied to the terminal. */
  droppedBytes: string;
  overDeletes: number;
  failures: string[];
  passed: boolean;
}

const isBoundary = (s: FixtureStep): s is Exclude<FixtureStep, MobileInputEvent> =>
  s.kind === "linkDown" || s.kind === "linkUp" || s.kind === "sessionSwitch";

function policyFor(f: ReplayFixture): MobileInputPolicy {
  return { ...DEFAULT_POLICY, ...f.policy };
}

export function replayFixture(f: ReplayFixture): ReplayResult {
  const policy = policyFor(f);
  const term = new ModelTerminal();
  const anomalies: string[] = [];
  let bytes = "";
  let droppedBytes = "";
  let linkUp = true;
  let state: MobileInputState = { kind: "idle", shadow: resetValue(policy), swallow: [], inputExpected: false };
  const deps = {
    getModifiers: () => NO_MODIFIERS,
    consumeModifiers: () => undefined,
    bracketedPasteEnabled: () => f.bracketedPaste === true,
  };

  for (const step of f.events) {
    if (isBoundary(step)) {
      if (step.kind === "linkDown") {
        linkUp = false;
        // Without a delivery ledger the text already handed over is the last known delivered prefix.
        if (state.kind === "composing" && state.mode === "live") state = demoteToDeferred(state, state.sent);
      } else if (step.kind === "linkUp") {
        linkUp = true;
      } else {
        state = abandonRegion(state);
      }
      continue;
    }
    const r = reduceWithKeymap(state, step, policy, deps);
    state = r.state;
    for (const c of r.commands) {
      if (c.kind === "Anomaly") anomalies.push(c.anomaly);
      if (c.kind !== "Emit") continue;
      if (linkUp) {
        bytes += c.bytes;
        term.apply(c.bytes);
      } else {
        droppedBytes += c.bytes;
      }
    }
  }

  const failures: string[] = [];
  if (bytes !== f.expectedBytes) {
    failures.push(`${f.id}: bytes ${JSON.stringify(bytes)} != expected ${JSON.stringify(f.expectedBytes)}`);
  }
  if (term.text !== f.finalText) {
    failures.push(`${f.id}: model line ${JSON.stringify(term.text)} != expected ${JSON.stringify(f.finalText)}`);
  }
  const wantAnomalies = f.expectedAnomalies ?? [];
  if (JSON.stringify(anomalies) !== JSON.stringify(wantAnomalies)) {
    failures.push(`${f.id}: anomalies ${JSON.stringify(anomalies)} != expected ${JSON.stringify(wantAnomalies)}`);
  }
  if (term.overDeletes > 0) failures.push(`${f.id}: ${term.overDeletes} DEL(s) ran past the start of the line`);

  return {
    id: f.id,
    source: f.source,
    bytes,
    finalText: term.text,
    anomalies,
    droppedBytes,
    overDeletes: term.overDeletes,
    failures,
    passed: failures.length === 0,
  };
}

/** One line, synthetic and device counted separately (ADR-007). */
export function summarize(results: readonly ReplayResult[]): string {
  const part = (source: ReplayFixture["source"]) => {
    const of = results.filter((r) => r.source === source);
    return `${of.filter((r) => r.passed).length}/${of.length}`;
  };
  return `synthetic ${part("synthetic")}, device ${part("device")}`;
}
