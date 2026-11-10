import { act, renderHook } from "@testing-library/react";
import { beforeinput, composition, ManualClock, restoreEnv, setAndroidEnv, setEnv, setValue } from "./domTestKit";
import { NO_MODIFIERS, type Modifiers } from "./keymap";
import { HIDDEN_INVALIDATE_MS, resetValue, DEFAULT_POLICY } from "./policy";
import { inputDiagnostics } from "./diagnostics";
import { suppressHelperTextarea } from "./rendererInputGuards";
import { useMobileInput, type MobileInputHost, type UseMobileInputOptions } from "./useMobileInput";
import type { MobileInputDiagnostic, SendResult } from "./types";

const RESET = resetValue(DEFAULT_POLICY);

interface Rig {
  container: HTMLElement;
  rendererElement: HTMLElement;
  surface: HTMLElement;
  sent: string[];
  diagnostics: MobileInputDiagnostic[];
  modifiers: { value: Modifiers; consumed: number };
  result: { value: SendResult | void };
  host: MobileInputHost;
  base: UseMobileInputOptions;
  clock: ManualClock;
  now: { value: number };
}

const rigs: Rig[] = [];

function makeRig(): Rig {
  const root = document.createElement("div");
  const rendererElement = document.createElement("div");
  const container = document.createElement("div");
  root.append(rendererElement, container);
  document.body.appendChild(root);
  const sent: string[] = [];
  const diagnostics: MobileInputDiagnostic[] = [];
  const modifiers = { value: NO_MODIFIERS, consumed: 0 };
  const result: Rig["result"] = { value: "sent" };
  const now = { value: 0 };
  const clock = new ManualClock();
  const host: MobileInputHost = {
    onData: (b) => (sent.push(b), result.value),
    getModifiers: () => modifiers.value,
    consumeModifiers: () => {
      modifiers.consumed++;
      modifiers.value = NO_MODIFIERS;
    },
    isReadOnly: () => false,
    onDiagnostic: (d) => diagnostics.push(d),
  };
  const base: UseMobileInputOptions = {
    enabled: true,
    visible: true,
    sessionId: "S1",
    isConnected: true,
    renderer: "xterm",
    getMount: () => ({ container, rendererElement, surface: rendererElement }),
    host,
    clock,
    now: () => now.value,
  };
  const rig = { container, rendererElement, surface: rendererElement, sent, diagnostics, modifiers, result, host, base, clock, now };
  rigs.push(rig);
  root.setAttribute("data-rig", String(rigs.length));
  return rig;
}

const layers = (r: Rig) => r.container.querySelectorAll('[data-testid="mobile-input-textarea"]');
const textarea = (r: Rig) => layers(r)[0] as HTMLTextAreaElement;

function typeHel(ta: HTMLTextAreaElement) {
  composition(ta, "compositionstart");
  for (const t of ["h", "he", "hel"]) {
    setValue(ta, RESET + t);
    beforeinput(ta, "insertCompositionText", t);
  }
}

beforeEach(() => {
  setAndroidEnv();
  jest.spyOn(console, "error").mockImplementation(() => undefined);
});
afterEach(() => {
  rigs.splice(0).forEach((r) => r.container.parentElement?.remove());
  restoreEnv();
  jest.restoreAllMocks();
});

describe("attach lifecycle", () => {
  it("hook_should_AttachWhenDockedAndDisposeWhenUndocked_When_VisibilityChanges", () => {
    const r = makeRig();
    const { rerender } = renderHook((p: UseMobileInputOptions) => useMobileInput(p), { initialProps: { ...r.base, visible: false } });
    expect(layers(r)).toHaveLength(0);
    rerender({ ...r.base, visible: true });
    expect(layers(r)).toHaveLength(1);
    rerender({ ...r.base, visible: false });
    expect(layers(r)).toHaveLength(0);
  });

  it("hook_should_AttachWithoutFocus_When_VisibleSoTapTargetExistsBeforeFirstTap", () => {
    const r = makeRig();
    renderHook(() => useMobileInput(r.base));
    expect(document.activeElement).not.toBe(textarea(r));
    // The first touch tap on the visible surface focuses the layer in the same tick.
    const up = new MouseEvent("pointerup", { bubbles: true, cancelable: true });
    Object.defineProperty(up, "pointerType", { value: "touch" });
    r.surface.dispatchEvent(up);
    expect(document.activeElement).toBe(textarea(r));
  });

  it("hook_should_DisposeController_When_TerminalHiddenOrUndocked", () => {
    const r = makeRig();
    const { unmount } = renderHook(() => useMobileInput(r.base));
    const ta = textarea(r);
    unmount();
    expect(ta.isConnected).toBe(false);
    expect(layers(r)).toHaveLength(0);
  });

  it("mount_should_CreateNoMobileInputTextarea_When_NonAndroidDevice", () => {
    setEnv({ coarse: false, hover: true, ua: "Mozilla/5.0 (X11; Linux x86_64)" });
    const r = makeRig();
    renderHook(() => useMobileInput(r.base));
    expect(layers(r)).toHaveLength(0);
    expect(r.container.childElementCount).toBe(0);
  });

  it("mount_should_CreateNoMobileInputTextarea_When_LayerFlagOff", () => {
    const r = makeRig();
    renderHook(() => useMobileInput({ ...r.base, enabled: false }));
    expect(r.container.childElementCount).toBe(0);
  });

  it("mount_should_PlaceLayerTextareaOutsideOpenElement_When_GhosttyOpenElementCarriesBlanketPreventDefault", () => {
    const r = makeRig();
    const blanket = jest.fn((e: Event) => e.preventDefault());
    r.rendererElement.addEventListener("beforeinput", blanket);
    r.rendererElement.addEventListener("keydown", blanket);
    renderHook(() => useMobileInput({ ...r.base, renderer: "ghostty" }));
    const ta = textarea(r);
    expect(r.rendererElement.contains(ta)).toBe(false);
    const e = beforeinput(ta, "insertText", "h");
    expect(blanket).not.toHaveBeenCalled();
    expect(e.defaultPrevented).toBe(false);
    expect(r.sent).toEqual(["h"]);
  });
});

describe("delivery", () => {
  it("hook_should_SendLayerBytesUnfilteredToOnData_When_OutputLooksLikeMouseReport", () => {
    const r = makeRig();
    renderHook(() => useMobileInput(r.base));
    beforeinput(textarea(r), "insertText", "\x1b[M abc");
    expect(r.sent).toEqual(["\x1b[M abc"]);
  });

  it("hook_should_PassStickyCtrlAsX03AndClearIt_When_SoftKeyboardC", () => {
    const r = makeRig();
    renderHook(() => useMobileInput(r.base));
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    beforeinput(textarea(r), "insertText", "c");
    expect(r.sent).toEqual(["\x03"]);
    expect(r.modifiers.consumed).toBe(1);
    expect(r.modifiers.value).toEqual(NO_MODIFIERS);
  });

  it("chain_should_PropagateDroppedFromSendInputToLayer_When_PoolOnDataReturnsDropped", () => {
    const r = makeRig();
    r.result.value = "dropped";
    renderHook(() => useMobileInput(r.base));
    beforeinput(textarea(r), "insertText", "x");
    expect(r.diagnostics).toContainEqual(expect.objectContaining({ event: "unit", result: "dropped" }));
  });

  it("hook_should_RecordClassAndLengthOnly_When_RingIsOpen", () => {
    const r = makeRig();
    inputDiagnostics.open();
    try {
      renderHook(() => useMobileInput(r.base));
      beforeinput(textarea(r), "insertText", "secret");
      const shown = inputDiagnostics.displayEvents();
      expect(JSON.stringify(shown)).not.toContain("secret");
      expect(shown.at(-1)).toMatchObject({ source: "emit", cls: "text", length: 6 });
    } finally {
      inputDiagnostics.close();
    }
  });
});

describe("renderer guards", () => {
  it("hook_should_SetAndRestoreHelperTextareaAttrs_When_AttachedThenDisposedOnXterm", () => {
    const r = makeRig();
    const helper = document.createElement("textarea");
    helper.setAttribute("tabindex", "0");
    r.rendererElement.appendChild(helper);
    const { unmount } = renderHook(() =>
      useMobileInput({ ...r.base, onAttached: () => suppressHelperTextarea(helper) }),
    );
    expect(helper.getAttribute("inputmode")).toBe("none");
    expect(helper.getAttribute("tabindex")).toBe("-1");
    unmount();
    expect(helper.hasAttribute("inputmode")).toBe(false);
    expect(helper.getAttribute("tabindex")).toBe("0");
  });

  it("mount_should_MakeGhosttyOwnTextareaInert_When_LayerActiveOnTouch", () => {
    const r = makeRig();
    const own = document.createElement("textarea");
    r.rendererElement.appendChild(own);
    own.focus();
    renderHook(() => useMobileInput({ ...r.base, renderer: "ghostty", onAttached: () => suppressHelperTextarea(own) }));
    expect(own.getAttribute("inputmode")).toBe("none");
    expect(own.getAttribute("tabindex")).toBe("-1");
    expect(document.activeElement).not.toBe(own);
  });
});

describe("focus", () => {
  it("handleFocus_should_FocusLayerTextarea_When_LayerActive", () => {
    const r = makeRig();
    const { result } = renderHook(() => useMobileInput(r.base));
    expect(result.current.focus()).toBe(true);
    expect(document.activeElement).toBe(textarea(r));
  });

  it("handleFocus_should_ReturnFalse_When_NoControllerSoCallerFallsBackToRenderer", () => {
    const r = makeRig();
    const { result } = renderHook(() => useMobileInput({ ...r.base, enabled: false }));
    expect(result.current.focus()).toBe(false);
    expect(result.current.blur()).toBe(false);
  });

  it("readOnlyTap_should_CallHostOnce_When_LayerFocusedInReadOnlySession", () => {
    const r = makeRig();
    const onReadOnlyTap = jest.fn();
    const host = { ...r.host, isReadOnly: () => true, onReadOnlyTap };
    const { result } = renderHook(() => useMobileInput({ ...r.base, host }));
    result.current.focus();
    expect(onReadOnlyTap).toHaveBeenCalledTimes(1);
  });

  it("readOnlyTap_should_NotCallHost_When_SessionWritable", () => {
    const r = makeRig();
    const onReadOnlyTap = jest.fn();
    const { result } = renderHook(() => useMobileInput({ ...r.base, host: { ...r.host, onReadOnlyTap } }));
    result.current.focus();
    expect(onReadOnlyTap).not.toHaveBeenCalled();
  });

  it("hook_should_FocusOnlyTerminalB_AndSetAInputmodeNone_When_TwoDockedTerminalsAndBGainsFocus", () => {
    const a = makeRig();
    const b = makeRig();
    renderHook(() => useMobileInput(a.base));
    const hb = renderHook(() => useMobileInput(b.base));
    hb.result.current.focus();
    expect(document.activeElement).toBe(textarea(b));
    expect(textarea(a).getAttribute("inputmode")).toBe("none");
    expect(textarea(b).hasAttribute("inputmode")).toBe(false);
  });

  it("hook_should_RestoreInputmode_When_FocusLeavesEveryLayer", () => {
    const a = makeRig();
    const b = makeRig();
    renderHook(() => useMobileInput(a.base));
    const hb = renderHook(() => useMobileInput(b.base));
    hb.result.current.focus();
    act(() => textarea(b).blur());
    expect(textarea(a).hasAttribute("inputmode")).toBe(false);
  });
});

describe("invalidate triggers", () => {
  const reasons = (r: Rig) => r.diagnostics.filter((d) => d.event === "invalidate").map((d) => (d as { reason: string }).reason);

  it("hook_should_CallInvalidateSession_When_SessionIdChangesMidComposition", () => {
    const r = makeRig();
    const { rerender } = renderHook((p: UseMobileInputOptions) => useMobileInput(p), { initialProps: r.base });
    typeHel(textarea(r));
    expect(r.sent.join("")).toBe("hel");
    rerender({ ...r.base, sessionId: "S2" });
    expect(reasons(r)).toEqual(["session"]);
    const before = r.sent.length;
    setValue(textarea(r), RESET + "yo");
    beforeinput(textarea(r), "insertCompositionText", "yo");
    expect(r.sent).toHaveLength(before);
  });

  it("hook_should_CallInvalidateTransport_When_IsConnectedFlipsEitherDirection", () => {
    const r = makeRig();
    const { rerender } = renderHook((p: UseMobileInputOptions) => useMobileInput(p), { initialProps: r.base });
    typeHel(textarea(r));
    rerender({ ...r.base, isConnected: false });
    rerender({ ...r.base, isConnected: true });
    expect(reasons(r)).toEqual(["transport", "transport"]);
  });

  it("hook_should_CallInvalidatePoolReuse_When_ReuseKeyChanges", () => {
    const r = makeRig();
    const { rerender } = renderHook((p: UseMobileInputOptions) => useMobileInput(p), { initialProps: { ...r.base, reuseKey: 1 } });
    rerender({ ...r.base, reuseKey: 2 });
    expect(reasons(r)).toEqual(["pool_reuse"]);
  });

  it("hook_should_NotInvalidate_When_PropsRerenderUnchanged", () => {
    const r = makeRig();
    const { rerender } = renderHook((p: UseMobileInputOptions) => useMobileInput(p), { initialProps: r.base });
    rerender({ ...r.base });
    expect(reasons(r)).toEqual([]);
  });

  describe("visibility", () => {
    const setVisibility = (state: "hidden" | "visible") => {
      Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
      document.dispatchEvent(new Event("visibilitychange"));
    };
    afterEach(() => setVisibility("visible"));

    it("hook_should_CallInvalidateVisibility_When_HiddenLongerThanBound", () => {
      const r = makeRig();
      renderHook(() => useMobileInput(r.base));
      setVisibility("hidden");
      r.now.value += HIDDEN_INVALIDATE_MS + 1;
      setVisibility("visible");
      expect(reasons(r)).toEqual(["visibility"]);
    });

    it("hook_should_NotInvalidate_When_HiddenBrieflyThenVisible", () => {
      const r = makeRig();
      renderHook(() => useMobileInput(r.base));
      setVisibility("hidden");
      r.now.value += HIDDEN_INVALIDATE_MS - 1;
      setVisibility("visible");
      expect(reasons(r)).toEqual([]);
    });
  });
});
