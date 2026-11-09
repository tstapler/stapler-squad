/** Tunables for the mobile input layer. Data, not logic, so a per-keyboard profile can swap them. */

/** Max graphemes of a composition region rewritten live; above it the region defers (ADR-002). */
export const MAX_REGION = 64;

/** UTF-8 byte ceiling for one atomic edit; larger payloads are paste/dictation (ADR-002). */
export const ATOMIC_EDIT_MAX_BYTES = 1024;

/** Idle time after which a composition with no events is force-ended. */
export const STUCK_COMPOSITION_MS = 2000;

/** Character the adapter keeps in the textarea so Backspace-on-empty still fires beforeinput. */
export const BACKSPACE_SENTINEL = "​";

export type BackspaceStrategy = "sentinel" | "keydown8";
export type WordDeleteStrategy = "ctrl-w" | "del-run";

export interface MobileInputPolicy {
  /**
   * How Backspace on an empty (reset) textarea reaches the terminal:
   * "sentinel" relies on BACKSPACE_SENTINEL giving the browser something to delete;
   * "keydown8" additionally honours keydown{key:"Backspace"} even with keyCode 229.
   * Switch to "keydown8" if the device probe shows no beforeinput on an empty textarea.
   */
  readonly backspaceStrategy: BackspaceStrategy;
  /** "del-run" replaces Ctrl-W with repeated DEL bounded by the word length (Story 0.1.3 fallback). */
  readonly wordDeleteStrategy: WordDeleteStrategy;
  readonly maxRegion: number;
  readonly atomicEditMaxBytes: number;
  readonly stuckCompositionMs: number;
}

export const DEFAULT_POLICY: MobileInputPolicy = {
  backspaceStrategy: "sentinel",
  wordDeleteStrategy: "ctrl-w",
  maxRegion: MAX_REGION,
  atomicEditMaxBytes: ATOMIC_EDIT_MAX_BYTES,
  stuckCompositionMs: STUCK_COMPOSITION_MS,
};

/** Text the textarea holds after a reset under the given strategy. */
export function resetValue(policy: MobileInputPolicy): string {
  return policy.backspaceStrategy === "sentinel" ? BACKSPACE_SENTINEL : "";
}

/** Textarea attributes (ADR-002 decision 5): autocorrect and spellcheck stay ON. */
export const TEXTAREA_ATTRIBUTES = {
  autocapitalize: "none",
  autocomplete: "off",
  enterkeyhint: "send",
} as const;
