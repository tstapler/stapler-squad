import { attachAndroidImeInput, isAndroidUserAgent } from "./androidImeInput";
import { attachImeTrace, imeTraceEnabled } from "./imeTrace";

export interface TerminalImeSupport {
  /** Wrap the terminal's own onData sink so the trace shows what the renderer emitted. */
  traceData: (data: string) => void;
  dispose: () => void;
}

/** Android IME shim (Android only) plus the [ime-trace] diagnostics (Android or ?imeTrace=1). */
export function attachTerminalImeSupport(
  root: HTMLElement,
  renderer: "xterm" | "ghostty",
  send: (data: string) => void,
): TerminalImeSupport {
  const trace = imeTraceEnabled() ? attachImeTrace(root, renderer) : undefined;
  const shimSend = (data: string) => {
    trace?.note(`shim-send len=${data.length}`);
    send(data);
  };
  const disposeShim = isAndroidUserAgent() ? attachAndroidImeInput(root, shimSend) : undefined;
  return {
    traceData: (data) => trace?.note(`onData len=${data.length}`),
    dispose: () => {
      disposeShim?.();
      trace?.dispose();
    },
  };
}
