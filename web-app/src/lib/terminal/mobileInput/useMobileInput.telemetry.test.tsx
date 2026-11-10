import { renderHook } from "@testing-library/react";
import { beforeinput, ManualClock, restoreEnv, setAndroidEnv } from "./domTestKit";
import { NO_MODIFIERS } from "./keymap";
import { INPUT_FLUSH_MS } from "./telemetry";
import { useMobileInput, type UseMobileInputOptions } from "./useMobileInput";
import type { SendResult } from "./types";
import { trackSharedEvent } from "@/lib/telemetry/rpcTiming";

jest.mock("@/lib/telemetry/rpcTiming", () => ({ trackSharedEvent: jest.fn() }));
const beacon = trackSharedEvent as jest.Mock;

function rig(renderer: "xterm" | "ghostty", result: SendResult | void = "sent") {
  const root = document.createElement("div");
  const container = document.createElement("div");
  const rendererElement = document.createElement("div");
  root.append(rendererElement, container);
  document.body.appendChild(root);
  const clock = new ManualClock();
  const opts: UseMobileInputOptions = {
    enabled: true,
    visible: true,
    sessionId: "S1",
    isConnected: true,
    renderer,
    getMount: () => ({ container, rendererElement, surface: rendererElement }),
    host: {
      onData: () => result,
      getModifiers: () => NO_MODIFIERS,
      consumeModifiers: () => undefined,
      isReadOnly: () => false,
    },
    clock,
  };
  const ta = () => container.querySelector('[data-testid="mobile-input-textarea"]') as HTMLTextAreaElement;
  return { opts, clock, ta, root };
}

beforeEach(() => {
  setAndroidEnv();
  beacon.mockClear();
});
afterEach(() => {
  document.body.innerHTML = "";
  restoreEnv();
});

describe("telemetry wiring", () => {
  it("hook_should_BeaconEmittedCountWithRendererLabel_When_FlushWindowElapses", () => {
    const r = rig("ghostty");
    renderHook(() => useMobileInput(r.opts));
    beforeinput(r.ta(), "insertText", "a");
    beforeinput(r.ta(), "insertText", "b");
    r.clock.advance(INPUT_FLUSH_MS);
    expect(beacon).toHaveBeenCalledWith({
      name: "cm.input",
      category: "performance",
      labels: { event: "emitted", renderer: "ghostty", count: "2" },
    });
  });

  it("hook_should_CountDropped_When_HostReportsDropped", () => {
    const r = rig("xterm", "dropped");
    renderHook(() => useMobileInput(r.opts));
    beforeinput(r.ta(), "insertText", "x");
    r.clock.advance(INPUT_FLUSH_MS);
    expect(beacon).toHaveBeenCalledWith(
      expect.objectContaining({ labels: { event: "dropped_disconnected", renderer: "xterm", count: "1" } }),
    );
  });

  it("hook_should_FlushPendingCountsAndContainNoTypedText_When_Unmounted", () => {
    const r = rig("xterm");
    const { unmount } = renderHook(() => useMobileInput(r.opts));
    beforeinput(r.ta(), "insertText", "hunter2-secret");
    expect(beacon).not.toHaveBeenCalled();
    unmount();
    expect(beacon).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(beacon.mock.calls)).not.toMatch(/hunter2|secret/);
  });
});
