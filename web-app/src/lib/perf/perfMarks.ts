const SWITCH = "ssq:window-switch";
const ROUTE = "ssq:route-change";
/** An input older than this is not treated as the cause of a route change (programmatic navigation). */
const INPUT_MAX_AGE_MS = 5000;

let switchPending = false;
let lastInputAt: number | null = null;
let inputCaptureInstalled = false;

function canMark(): boolean {
  return typeof performance !== "undefined" && typeof performance.mark === "function";
}

/** Starts the `ssq:window-switch` measure; call synchronously in the switch action. */
export function markWindowSwitchStart(): void {
  if (!canMark()) return;
  performance.mark(`${SWITCH}:start`);
  switchPending = true;
}

/** Ends the measure after two animation frames (one committed + one painted frame), so it names "interactive". */
export function markWindowSwitchEnd(): void {
  if (!switchPending || !canMark()) return;
  switchPending = false;
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      performance.mark(`${SWITCH}:end`);
      performance.measure(SWITCH, `${SWITCH}:start`, `${SWITCH}:end`);
    })
  );
}

/** Records the latest user input time, the start of any route-change measure it triggers. Idempotent. */
export function installInputStartCapture(): void {
  if (inputCaptureInstalled || typeof document === "undefined") return;
  inputCaptureInstalled = true;
  const record = () => {
    lastInputAt = performance.now();
  };
  document.addEventListener("pointerdown", record, true);
  document.addEventListener("keydown", record, true);
}

/** Records `ssq:route-change` from the last user input to now; no-op for navigations with no recent input. */
export function markRouteChangeEnd(): void {
  if (!canMark() || lastInputAt === null) return;
  const start = lastInputAt;
  lastInputAt = null;
  if (performance.now() - start > INPUT_MAX_AGE_MS) return;
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      performance.measure(ROUTE, { start, end: performance.now() });
    })
  );
}

/** Test-only: clear module state. */
export function _resetPerfMarksForTesting(): void {
  switchPending = false;
  lastInputAt = null;
}
