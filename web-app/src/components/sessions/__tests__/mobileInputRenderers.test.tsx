// @feature terminal-input
/**
 * One parametrized test over BOTH renderers (Story 1.4.2): the real XtermTerminal and GhosttyTerminal wrappers
 * over faked terminals deliver identical bytes for the replay corpus. The fake is not ghostty-web: real focus,
 * IME and keyCode 229 behaviour is covered only by the Playwright spec (Story 5.1.1).
 */
import "./mobileInputTestSetup";
import { act, render, screen, waitFor } from "@testing-library/react";
import { dispatchStep, beforeinput, ManualClock } from "@/lib/terminal/mobileInput/domTestKit";
import { loadFixtures } from "@/lib/terminal/mobileInput/fixtureCorpus";
import { NO_MODIFIERS } from "@/lib/terminal/mobileInput/keymap";
import type { MobileInputHost, MobileInputRendererProps } from "@/lib/terminal/mobileInput/useMobileInput";
import { Terminal as FakeTerminal } from "@xterm/xterm";
import { GhosttyTerminal } from "../GhosttyTerminal";
import { XtermTerminal, type XtermTerminalHandle } from "../XtermTerminal";
import { resetEnv, useAndroidLayerEnv, useDesktopEnv } from "./mobileInputTestSetup";
import { createRef } from "react";

type FakeTerm = { textarea?: HTMLTextAreaElement; opened?: HTMLElement };
type FakeClass = { instances: FakeTerm[]; received: string[] };

const RENDERERS = {
  xterm: { Component: XtermTerminal, fake: async () => FakeTerminal as unknown as FakeClass },
  ghostty: { Component: GhosttyTerminal, fake: async () => (await import("ghostty-web")).Terminal as unknown as FakeClass },
};

function must<T>(value: T | null | undefined, what: string): T {
  if (value === null || value === undefined) throw new Error(`missing ${what}`);
  return value;
}

const hello = must(loadFixtures().find((f) => f.id === "gboard-typing-hello"), "fixture gboard-typing-hello");

function mount(kind: keyof typeof RENDERERS, over: Partial<MobileInputRendererProps> = {}) {
  const sent: string[] = [];
  const host: MobileInputHost = {
    onData: () => "dropped",
    getModifiers: () => NO_MODIFIERS,
    consumeModifiers: () => undefined,
    isReadOnly: () => false,
  };
  const handle = createRef<XtermTerminalHandle>();
  const { Component } = RENDERERS[kind];
  const props = { host, sessionId: "S1", isConnected: true, visible: true, reuseKey: 1, ...over };
  render(<Component ref={handle} onData={(d) => (sent.push(d), "sent")} mobileInput={props} />);
  return { sent, handle };
}

const layerTextarea = () => screen.getByTestId("mobile-input-textarea") as HTMLTextAreaElement;

afterEach(() => {
  resetEnv();
  jest.restoreAllMocks();
});

describe.each(Object.keys(RENDERERS) as Array<keyof typeof RENDERERS>)("%s renderer", (kind) => {
  const lastTerminal = async () => must((await RENDERERS[kind].fake()).instances.at(-1), "terminal");
  const ownTextarea = async () => must((await lastTerminal()).textarea, "renderer textarea");

  describe("on the Android class with the layer on", () => {
    beforeEach(useAndroidLayerEnv);

    it("renderers_should_DeliverIdenticalHelloBytes_When_GboardTypingFixtureDispatched", async () => {
      const { sent } = mount(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      const clock = new ManualClock();
      const fake = await RENDERERS[kind].fake();
      fake.received.length = 0;
      for (const step of hello.events) dispatchStep(layerTextarea(), step, clock, () => undefined);
      expect(sent.join("")).toBe("hello");
      expect(fake.received).toEqual([]); // the renderer's open() element saw no layer events
    });

    it("renderers_should_PlaceLayerTextareaBesideRendererElement_When_Mounted", async () => {
      mount(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      const term = await lastTerminal();
      expect(term.opened?.contains(layerTextarea())).toBe(false);
    });

    it("renderers_should_NotDoubleSend_When_FocusBouncesToRendererTextarea", async () => {
      const { sent } = mount(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      beforeinput(layerTextarea(), "insertText", "a");
      const own = await ownTextarea();
      own.focus(); // focus bounces to the renderer's own textarea
      own.dispatchEvent(new KeyboardEvent("keydown", { key: "b", bubbles: true }));
      layerTextarea().focus();
      beforeinput(layerTextarea(), "insertText", "c");
      expect(sent).toEqual(["a", "b", "c"]);
    });

    it("renderers_should_MakeOwnTextareaInert_When_LayerActive", async () => {
      mount(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      // ghostty-web opens asynchronously, so its own textarea is made inert once it exists.
      await waitFor(async () => {
        const own = (await RENDERERS[kind].fake()).instances.at(-1)?.textarea;
        expect(own?.getAttribute("inputmode")).toBe("none");
        expect(own?.getAttribute("tabindex")).toBe("-1");
      });
    });

    it("handle_should_FocusLayerTextarea_When_LayerActive", async () => {
      const { handle } = mount(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      act(() => must(handle.current, "handle").focus());
      expect(document.activeElement).toBe(layerTextarea());
    });

    it("renderers_should_DisposeLayerAndRestoreOwnTextarea_When_Undocked", async () => {
      const { rerender } = renderUndockable(kind);
      await waitFor(() => expect(layerTextarea()).toBeInTheDocument());
      const own = await ownTextarea();
      rerender(false);
      expect(screen.queryByTestId("mobile-input-textarea")).toBeNull();
      expect(own.hasAttribute("inputmode")).toBe(false);
    });
  });

  describe("when the layer cannot attach (no beforeinput support)", () => {
    const desc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "onbeforeinput");
    beforeEach(() => {
      useAndroidLayerEnv();
      delete (HTMLElement.prototype as unknown as Record<string, unknown>).onbeforeinput;
    });
    afterEach(() => {
      if (desc) Object.defineProperty(HTMLElement.prototype, "onbeforeinput", desc);
    });

    it("renderers_should_LeaveOwnTextareaUntouched_When_AttachReturnsNull", async () => {
      mount(kind);
      await waitFor(async () => expect((await RENDERERS[kind].fake()).instances.at(-1)?.textarea).toBeDefined());
      await act(async () => {});
      const own = await ownTextarea();
      expect(screen.queryByTestId("mobile-input-textarea")).toBeNull();
      expect(own.hasAttribute("inputmode")).toBe(false);
      expect(own.hasAttribute("tabindex")).toBe(false);
    });

    it("handle_should_FallBackToRendererFocus_When_AttachReturnsNull", async () => {
      const { handle } = mount(kind);
      await waitFor(async () => expect((await RENDERERS[kind].fake()).instances.at(-1)?.textarea).toBeDefined());
      await act(async () => {});
      const own = await ownTextarea();
      act(() => must(handle.current, "handle").focus());
      expect(document.activeElement).toBe(own);
    });
  });

  describe("without the layer", () => {
    it("mount_should_CreateNoMobileInputTextarea_When_NonAndroidDevice", async () => {
      useDesktopEnv();
      mount(kind);
      await act(async () => {});
      expect(screen.queryByTestId("mobile-input-textarea")).toBeNull();
      expect(document.querySelector("[data-mobile-input-host]")).toBeNull();
    });

    it("handle_should_FocusRendererItself_When_NoLayer", async () => {
      useDesktopEnv();
      const { handle } = mount(kind);
      await act(async () => {});
      const own = await ownTextarea();
      act(() => must(handle.current, "handle").focus());
      expect(document.activeElement).toBe(own);
    });
  });
});

/** Re-renders one renderer with `visible` toggled, as the pool does when a session is undocked. */
function renderUndockable(kind: keyof typeof RENDERERS) {
  const { Component } = RENDERERS[kind];
  const host: MobileInputHost = {
    onData: () => "dropped",
    getModifiers: () => NO_MODIFIERS,
    consumeModifiers: () => undefined,
    isReadOnly: () => false,
  };
  const ui = (visible: boolean) => (
    <Component onData={() => "sent"} mobileInput={{ host, sessionId: "S1", isConnected: true, visible, reuseKey: 1 }} />
  );
  const view = render(ui(true));
  return { rerender: (visible: boolean) => view.rerender(ui(visible)) };
}
