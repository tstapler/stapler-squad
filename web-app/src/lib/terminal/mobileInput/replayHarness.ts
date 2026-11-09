/** Replays a fixture trace through the pure engine into a model terminal. No DOM, no real clock. */
import type { FixtureStep, ReplayFixture } from "./fixtureSchema";
import { InputEngine } from "./inputEngine";
import { NO_MODIFIERS } from "./keymap";
import { DEFAULT_POLICY, type MobileInputPolicy } from "./policy";
import { graphemes } from "./rewrite";
import type { MobileInputEvent, SendResult } from "./types";

/** Terminal line as a hardware keyboard would leave it: DEL erases a grapheme, ^W a word, text appends. */
export class ModelTerminal {
  text: string;
  /** DELs that would have eaten the prompt (or hit an empty line). */
  overDeletes = 0;

  constructor(private readonly prompt = "") {
    this.text = prompt;
  }

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

  /** The line after the prompt. */
  get typed(): string {
    return this.text.slice(this.prompt.length);
  }

  private erase(unit: "grapheme" | "word"): void {
    if (this.text.length <= this.prompt.length) {
      this.overDeletes++;
      return;
    }
    const kept = unit === "grapheme" ? graphemes(this.text).slice(0, -1).join("") : this.text.replace(/\S+\s*$/u, "");
    this.text = kept.length < this.prompt.length ? this.prompt : kept;
  }
}

/** Samples the line once per animation frame; units landing in the same frame are one sample. */
export class FrameSampler {
  private frames: Array<{ frame: number; text: string }> = [];
  private offset = 0;

  constructor(private readonly frameMs = 16) {}

  record(t: number, text: string): void {
    const frame = Math.floor(t / this.frameMs) + this.offset;
    const last = this.frames[this.frames.length - 1];
    if (last && last.frame === frame) last.text = text;
    else this.frames.push({ frame, text });
  }

  /** The next record lands in a new frame even if `t` has not advanced. */
  nextFrame(): void {
    this.offset++;
  }

  /** Line text at the end of each sampled frame, consecutive repeats collapsed. */
  get lines(): string[] {
    return this.frames.map((f) => f.text).filter((text, i, all) => i === 0 || text !== all[i - 1]);
  }
}

/** Distinct lines sampled from the pre-rewrite frame onward that equal neither `pre` nor `post`. */
export function intermediateLines(lines: readonly string[], pre: string, post: string): string[] {
  const from = lines.indexOf(pre);
  const window = from < 0 ? lines : lines.slice(from);
  return [...new Set(window.filter((l) => l !== pre && l !== post))];
}

/**
 * Oracle for the `no-DEL-across-boundary` invariant: a DEL may only erase graphemes the layer emitted in
 * the current epoch and session, and no byte crosses a session invalidate until the stale region ends.
 */
export class DelBoundaryOracle {
  violations: string[] = [];
  private eligible: string[] = [];
  private staleSession = false;

  /**
   * Call for every unit handed to the sink; `accepted` is false when the sink refused it. `rewrite` is
   * false for units a deliberate user key produced (a Backspace erases terminal text, not layer text).
   */
  onUnit(bytes: string, accepted: boolean, rewrite = true): void {
    const dels = [...bytes].filter((c) => c === "\x7f").length;
    if (rewrite && this.staleSession) this.violations.push("bytes crossed a session invalidate");
    if (rewrite && dels > this.eligible.length) {
      this.violations.push(`${dels} DEL(s) but only ${this.eligible.length} grapheme(s) emitted this epoch`);
    }
    if (!accepted) {
      this.eligible = [];
      return;
    }
    const body = bytes.replace(/\x7f/g, "");
    this.eligible = [...this.eligible.slice(0, Math.max(0, this.eligible.length - dels)), ...graphemes(body)];
  }

  /** A transport boundary or session switch: earlier text can no longer be erased. */
  boundary(): void {
    this.eligible = [];
  }

  sessionSwitch(composing: boolean): void {
    this.eligible = [];
    this.staleSession = composing;
  }

  /** The region committed; the next composition starts from an empty base. */
  compositionEnded(): void {
    this.eligible = [];
    this.staleSession = false;
  }
}

export interface ReplayOptions {
  /** Text already on the line before the trace; DELs into it count as over-deletes. */
  prompt?: string;
  sampler?: FrameSampler;
  /** Test-only: land every byte of a unit in its own frame, as a non-atomic sink would. */
  framePerByte?: boolean;
}

export interface ReplayResult {
  id: string;
  source: ReplayFixture["source"];
  /** Every unit the sink accepted, including frames the model terminal never received. */
  bytes: string;
  finalText: string;
  anomalies: string[];
  /** Units the sink refused; never applied to the terminal. */
  droppedBytes: string;
  overDeletes: number;
  invariantViolations: string[];
  failures: string[];
  passed: boolean;
}

type Boundary = Exclude<FixtureStep, MobileInputEvent>;
const isBoundary = (s: FixtureStep): s is Boundary =>
  s.kind === "linkDown" || s.kind === "linkUp" || s.kind === "sessionSwitch" || s.kind === "dropNext" || s.kind === "loseNext";

/** Events that move a composition region (as opposed to a deliberate Backspace, Enter or plain key). */
const isRegionEvent = (e: MobileInputEvent): boolean =>
  e.kind === "compositionupdate" ||
  e.kind === "compositionend" ||
  e.kind === "tick" ||
  e.kind === "blur" ||
  (e.kind === "beforeinput" && (e.inputType === "insertCompositionText" || e.inputType === "insertReplacementText"));

export function replayFixture(f: ReplayFixture, options: ReplayOptions = {}): ReplayResult {
  const policy: MobileInputPolicy = { ...DEFAULT_POLICY, ...f.policy };
  const term = new ModelTerminal(options.prompt ?? "");
  const oracle = new DelBoundaryOracle();
  const anomalies: string[] = [];
  let bytes = "";
  let droppedBytes = "";
  let linkUp = true;
  let dropNext = 0;
  let loseNext = 0;
  let now = 0;
  let rewriteStep = true;

  const land = (unit: string) => {
    if (options.framePerByte) {
      for (const ch of unit) {
        term.apply(ch);
        options.sampler?.record(now, term.text);
        options.sampler?.nextFrame();
      }
      return;
    }
    term.apply(unit);
    options.sampler?.record(now, term.text);
  };

  const engine = new InputEngine({
    policy,
    deps: {
      getModifiers: () => NO_MODIFIERS,
      consumeModifiers: () => undefined,
      bracketedPasteEnabled: () => f.bracketedPaste === true,
    },
    isReadOnly: () => false,
    onAnomaly: (a) => anomalies.push(a),
    onData: (unit): SendResult => {
      const refused = !linkUp || dropNext > 0;
      if (dropNext > 0) dropNext--;
      oracle.onUnit(unit, !refused, rewriteStep);
      if (refused) {
        droppedBytes += unit;
        return "dropped";
      }
      bytes += unit;
      if (loseNext > 0) loseNext--;
      else land(unit);
      return "sent";
    },
  });

  for (const step of f.events) {
    if (!isBoundary(step)) {
      now = step.t;
      rewriteStep = isRegionEvent(step);
      engine.dispatch(step);
      if (step.kind === "compositionend") oracle.compositionEnded();
      continue;
    }
    switch (step.kind) {
      case "linkDown":
        linkUp = false;
        oracle.boundary();
        engine.invalidate("transport");
        break;
      case "linkUp":
        linkUp = true;
        oracle.boundary();
        engine.invalidate("transport");
        break;
      case "sessionSwitch":
        oracle.sessionSwitch(engine.state.kind === "composing");
        engine.invalidate("session");
        break;
      case "dropNext":
        dropNext++;
        break;
      case "loseNext":
        loseNext++;
        break;
    }
  }

  const failures: string[] = [];
  if (bytes !== f.expectedBytes) {
    failures.push(`${f.id}: bytes ${JSON.stringify(bytes)} != expected ${JSON.stringify(f.expectedBytes)}`);
  }
  if (term.typed !== f.finalText) {
    failures.push(`${f.id}: model line ${JSON.stringify(term.typed)} != expected ${JSON.stringify(f.finalText)}`);
  }
  const wantAnomalies = f.expectedAnomalies ?? [];
  if (JSON.stringify(anomalies) !== JSON.stringify(wantAnomalies)) {
    failures.push(`${f.id}: anomalies ${JSON.stringify(anomalies)} != expected ${JSON.stringify(wantAnomalies)}`);
  }
  if (term.overDeletes > 0) failures.push(`${f.id}: ${term.overDeletes} DEL(s) ran past the start of the line`);
  for (const v of oracle.violations) failures.push(`${f.id}: no-DEL-across-boundary violated: ${v}`);

  return {
    id: f.id,
    source: f.source,
    bytes,
    finalText: term.typed,
    anomalies,
    droppedBytes,
    overDeletes: term.overDeletes,
    invariantViolations: oracle.violations,
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
