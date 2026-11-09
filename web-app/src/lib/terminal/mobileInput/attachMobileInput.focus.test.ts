import { attachMobileInput, type AttachOptions, type MobileInputController } from "./attachMobileInput";
import { beforeinput, ManualClock, restoreEnv, setAndroidEnv } from "./domTestKit";
import { NO_MODIFIERS } from "./keymap";
import { TAP_DEDUPE_MS } from "./policy";

let cleanup: Array<() => void> = [];

function setup() {
  const host = document.createElement("div");
  const surface = document.createElement("div");
  host.appendChild(surface);
  document.body.appendChild(host);
  const clock = new ManualClock();
  const sent: string[] = [];
  const base: AttachOptions = {
    onData: (b) => (sent.push(b), "sent"),
    getModifiers: () => NO_MODIFIERS,
    consumeModifiers: () => undefined,
    isReadOnly: () => false,
    onAnomaly: () => undefined,
    clock,
    rendererElement: surface,
  };
  const c = attachMobileInput(host, base) as MobileInputController;
  const focusSpy = jest.spyOn(c.element, "focus");
  cleanup.push(() => {
    c.dispose();
    host.remove();
  });
  return { host, surface, clock, c, sent, focusSpy };
}

/** jsdom has no PointerEvent: a MouseEvent of that type with the fields the layer reads. */
function pointer(type: string, init: { pointerType: string; x?: number; y?: number }): Event {
  const e = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: init.x ?? 0, clientY: init.y ?? 0 });
  Object.defineProperty(e, "pointerType", { value: init.pointerType });
  return e;
}

function touch(type: "touchstart" | "touchend", x = 0, y = 0): Event {
  const e = new Event(type, { bubbles: true, cancelable: true });
  const t = { clientX: x, clientY: y };
  Object.defineProperty(e, "changedTouches", { value: [t] });
  Object.defineProperty(e, "touches", { value: type === "touchend" ? [] : [t] });
  return e;
}

beforeEach(() => {
  setAndroidEnv();
  jest.spyOn(console, "error").mockImplementation(() => undefined);
});
afterEach(() => {
  cleanup.forEach((f) => f());
  cleanup = [];
  restoreEnv();
  jest.restoreAllMocks();
});

describe("controller focus", () => {
  it("controller_should_FocusTextareaSynchronously_When_FocusCalledInsideClickHandler", () => {
    const { c, surface } = setup();
    surface.addEventListener("click", () => {
      c.focus();
      expect(document.activeElement).toBe(c.element);
    });
    surface.click();
    expect(document.activeElement).toBe(c.element);
    expect(c.focused).toBe(true);
  });

  it("controller_should_BlurTextarea_When_BlurCalledAfterSelection", () => {
    const { c } = setup();
    c.focus();
    c.blur();
    expect(document.activeElement).not.toBe(c.element);
    expect(c.focused).toBe(false);
  });

  it("controller_should_BeInactiveAndIgnoreFocus_When_Disposed", () => {
    const { c } = setup();
    expect(c.active).toBe(true);
    c.dispose();
    expect(c.active).toBe(false);
    c.focus();
    expect(document.activeElement).not.toBe(c.element);
  });

  it("toolbarCtrl_should_KeepTextareaActive_When_PointerDownPreventDefault", () => {
    const { c, host } = setup();
    const btn = document.createElement("button");
    btn.addEventListener("pointerdown", (e) => e.preventDefault());
    btn.addEventListener("mousedown", (e) => e.preventDefault());
    host.appendChild(btn);
    c.focus();
    btn.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(c.element);
  });
});

describe("bindTapTarget", () => {
  it("bindTapTarget_should_FocusSynchronouslyOnce_When_PointerupTouchendClickAndGestureHandlersPreventDefault", () => {
    const { c, surface, focusSpy } = setup();
    c.bindTapTarget(surface);
    // A gesture hook that swallows touches and stops propagation after the layer saw them.
    for (const type of ["touchstart", "touchend", "pointerup"]) {
      surface.addEventListener(type, (e) => e.preventDefault());
    }
    surface.dispatchEvent(touch("touchstart", 5, 5));
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch", x: 5, y: 5 }));
    expect(document.activeElement).toBe(c.element);
    surface.dispatchEvent(touch("touchend", 5, 5));
    surface.click();
    expect(focusSpy).toHaveBeenCalledTimes(1);
  });

  it("bindTapTarget_should_FocusOnce_When_PointercancelThenClick", () => {
    const { c, surface, focusSpy } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(pointer("pointerdown", { pointerType: "touch" }));
    surface.dispatchEvent(pointer("pointercancel", { pointerType: "touch" }));
    expect(focusSpy).not.toHaveBeenCalled();
    surface.click();
    expect(document.activeElement).toBe(c.element);
    expect(focusSpy).toHaveBeenCalledTimes(1);
  });

  it("bindTapTarget_should_FocusViaTouchendAlone_When_PointerEventsNeverFire", () => {
    const { c, surface } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(touch("touchend"));
    expect(document.activeElement).toBe(c.element);
  });

  it("bindTapTarget_should_IgnoreMousePointerup_When_PointerTypeMouse", () => {
    const { c, surface, focusSpy } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(pointer("pointerup", { pointerType: "mouse" }));
    expect(focusSpy).not.toHaveBeenCalled();
  });

  it("bindTapTarget_should_NotFocus_When_TouchTravelsPastSlop", () => {
    const { c, surface, focusSpy } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(touch("touchstart", 0, 0));
    surface.dispatchEvent(touch("touchend", 0, 80));
    expect(focusSpy).not.toHaveBeenCalled();
  });

  it("bindTapTarget_should_FocusAgain_When_SecondTapAfterDedupeWindow", () => {
    const { c, surface, clock, focusSpy } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    clock.advance(TAP_DEDUPE_MS + 1);
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(focusSpy).toHaveBeenCalledTimes(2);
  });

  it("bindTapTarget_should_RefocusLayer_When_RendererTakesFocusAfterTouchendWithinDedupeWindow", () => {
    const { c, surface } = setup();
    const rendererTextarea = document.createElement("textarea");
    surface.appendChild(rendererTextarea);
    c.bindTapTarget(surface);
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(document.activeElement).toBe(c.element);
    // The compat mousedown after touchend makes the renderer focus its own helper textarea.
    rendererTextarea.focus();
    expect(document.activeElement).toBe(rendererTextarea);
    surface.click();
    expect(document.activeElement).toBe(c.element);
  });

  it("bindTapTarget_should_FocusLayerOnGhosttyCanvasContainer_When_TouchPointerupLands", () => {
    const { c, surface } = setup();
    const canvas = document.createElement("canvas");
    surface.appendChild(canvas);
    c.bindTapTarget(surface);
    canvas.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(document.activeElement).toBe(c.element);
  });

  it("bindTapTarget_should_StopFocusing_When_UnboundOrDisposed", () => {
    const { c, surface, focusSpy } = setup();
    const unbind = c.bindTapTarget(surface);
    unbind();
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(focusSpy).not.toHaveBeenCalled();
    c.bindTapTarget(surface);
    c.dispose();
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(focusSpy).not.toHaveBeenCalled();
  });

  it("bindTapTarget_should_StayInert_When_AskedToBindTheTextareaItself", () => {
    const { c, focusSpy } = setup();
    c.bindTapTarget(c.element);
    c.element.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    expect(focusSpy).not.toHaveBeenCalled();
  });

  it("controller_should_KeepTypingWorking_When_FocusedByTap", () => {
    const { c, surface, sent } = setup();
    c.bindTapTarget(surface);
    surface.dispatchEvent(pointer("pointerup", { pointerType: "touch" }));
    beforeinput(c.element, "insertText", "a");
    expect(sent).toEqual(["a"]);
  });
});
