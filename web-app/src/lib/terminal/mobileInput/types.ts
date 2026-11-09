/** Pure types for the mobile input layer. No DOM, no React. */

declare const brand: unique symbol;
type Brand<T, B extends string> = T & { readonly [brand]: B };

/** Exact string this layer already emitted for the current composition region. */
export type SentText = Brand<string, "SentText">;
/** Text of the current composition as the browser reports it. */
export type CompositionRegion = Brand<string, "CompositionRegion">;
/** String handed to onData; may contain DEL and text. Never log it. */
export type TerminalBytes = Brand<string, "TerminalBytes">;
/** Monotonic id the layer assigns to each emitted atomic unit. */
export type UnitSeq = Brand<number, "UnitSeq">;

export const asSentText = (s: string): SentText => s as SentText;
export const asRegion = (s: string): CompositionRegion => s as CompositionRegion;
export const asTerminalBytes = (s: string): TerminalBytes => s as TerminalBytes;
export const asUnitSeq = (n: number): UnitSeq => n as UnitSeq;

export type SendResult = "sent" | "queued" | "dropped";
export type OnData = (bytes: TerminalBytes, seq: UnitSeq) => SendResult | void;

export type ImeAnomaly =
  | "orphaned_input"
  | "rewrite_bound_exceeded"
  | "unrecognized_sequence"
  | "stuck_composition"
  | "selection_in_region"
  | "duplicate_suppressed";

export type MobileInputEvent =
  | { kind: "keydown"; key: string; keyCode: number; isComposing: boolean; t: number }
  | {
      kind: "beforeinput";
      inputType: string;
      data: string | null;
      value: string;
      selStart: number;
      selEnd: number;
      t: number;
    }
  | { kind: "input"; value: string; selStart: number; selEnd: number; t: number }
  | { kind: "compositionstart"; t: number }
  | { kind: "compositionupdate"; data: string; t: number }
  | { kind: "compositionend"; data: string; t: number }
  | { kind: "selectionchange"; selStart: number; selEnd: number; t: number }
  | { kind: "blur"; t: number }
  | { kind: "tick"; t: number };

/** An echo the next event may repeat; swallowed once so bytes are never sent twice. */
export type Swallow =
  | { kind: "compositionend" }
  | { kind: "insertText"; data: string }
  | { kind: "deleteBackward" }
  | { kind: "lineBreak" };

export type MobileInputState =
  | {
      kind: "idle";
      /** What the textarea holds after a reset ("" or the sentinel). */
      shadow: string;
      swallow: readonly Swallow[];
      /** True after a beforeinput/compositionend that an `input` event should follow. */
      inputExpected: boolean;
    }
  | {
      kind: "composing";
      region: CompositionRegion;
      sent: SentText;
      /** "deferred": hold text until commit, emit an insert-only tail, never a DEL. "abandoned": emit nothing (ledger invalidated the region). */
      mode: "live" | "deferred" | "abandoned";
      lastT: number;
    }
  | { kind: "degraded"; shadow: string };

export type InputCommand =
  | {
      kind: "Emit";
      bytes: TerminalBytes;
      /** UTF-8 length below ATOMIC_EDIT_MAX_BYTES: deliver as one sendInput, no paste chunking. */
      atomic: boolean;
      paste?: true;
    }
  | { kind: "ResetTextarea" }
  | { kind: "Anomaly"; anomaly: ImeAnomaly }
  | { kind: "EnterDeferred" };

export interface ReduceResult {
  state: MobileInputState;
  commands: readonly InputCommand[];
  /** True only for keydown events the layer fully handled. */
  preventDefault: boolean;
}

export type RewriteEdit =
  | { kind: "edit"; backspaces: number; insert: string }
  | { kind: "deferred"; anomaly: "rewrite_bound_exceeded" };
