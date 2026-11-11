/**
 * Shared setup for the mobile input renderer tests: ONE place that fakes the xterm.js and ghostty-web
 * modules (no WASM, canvas or WebGL in jsdom) and puts the page on the Android class with the layer flag on.
 * Import it BEFORE any renderer so the module mocks are registered first. Real ghostty-web behaviour (focus,
 * IME, keyCode 229) is covered only by the Playwright spec (Story 5.1.1); a fake cannot prove it.
 */
import { setAndroidEnv, setEnv, restoreEnv } from "@/lib/terminal/mobileInput/domTestKit";

const mockStubAddon = (name: string, members: Record<string, unknown> = {}) => ({
  [name]: class {
    constructor() {
      Object.assign(this, members);
    }
  },
});

jest.mock("@xterm/addon-fit", () => mockStubAddon("FitAddon", { fit: () => undefined, proposeDimensions: () => undefined }));
jest.mock("@xterm/addon-webgl", () => mockStubAddon("WebglAddon", { dispose: () => undefined, onContextLoss: () => undefined }));
jest.mock("@xterm/addon-canvas", () => mockStubAddon("CanvasAddon", { dispose: () => undefined }));
jest.mock("@xterm/addon-web-links", () => mockStubAddon("WebLinksAddon"));
jest.mock("@xterm/addon-unicode11", () => mockStubAddon("Unicode11Addon"));
jest.mock("@xterm/addon-serialize", () => mockStubAddon("SerializeAddon"));
jest.mock("@xterm/addon-search", () => mockStubAddon("SearchAddon", { findNext: () => true, findPrevious: () => true }));

jest.mock("@xterm/xterm", () => ({ Terminal: require("./fakeTerminal").fakeTerminalSource() }));
jest.mock("ghostty-web", () => {
  const noopAddon = class {
    fit = () => undefined;
    observeResize = () => undefined;
    dispose = () => undefined;
  };
  return { init: async () => undefined, Terminal: require("./fakeTerminal").fakeTerminalSource(), FitAddon: noopAddon };
});

export const LAYER_FLAG_KEY = "ssq:flag:mobile_input_layer";

/** The env helpers answer matchMedia with `{matches}` only; the real renderers also subscribe to changes. */
function withMediaListeners(): void {
  const answer = window.matchMedia;
  window.matchMedia = ((q: string) => ({
    ...answer(q),
    media: q,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  })) as unknown as typeof window.matchMedia;
}

export function useAndroidLayerEnv(): void {
  localStorage.setItem(LAYER_FLAG_KEY, "1");
  setAndroidEnv();
  withMediaListeners();
}

export function useDesktopEnv(): void {
  localStorage.setItem(LAYER_FLAG_KEY, "1");
  setEnv({ coarse: false, hover: true, ua: "Mozilla/5.0 (X11; Linux x86_64) Chrome/120" });
  withMediaListeners();
}

export function resetEnv(): void {
  localStorage.clear();
  restoreEnv();
}
