/**
 * Ground-truth verification: does the real, unmocked `@xterm/addon-canvas@0.7.0`
 * (the WebGL->Canvas fallback's CanvasAddon, see XtermTerminal.tsx and ADR-001 at
 * project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md)
 * still work against the real, unmocked `@xterm/xterm@6.0.0` installed in this repo?
 *
 * Why this test exists: `@xterm/addon-canvas@0.7.0`'s package.json declares a peer
 * dependency of `@xterm/xterm: ^5.0.0`. `@xterm/xterm` was bumped to `6.0.0` in an
 * unrelated PR after this dependency was added (ADR-001), and upstream xterm.js
 * removed the canvas renderer from its own monorepo entirely as of 6.0.0
 * (xtermjs/xterm.js#5105: "Remove the canvas renderer -- this addon no longer
 * exists and we recommend using either the DOM renderer or WebGL"). `pnpm install`
 * only *warns* on the unmet peer range rather than failing (no `strict-peer-dependencies`
 * config in this repo), so CI stays green either way -- but that green tells you
 * nothing about whether `CanvasAddon` actually still functions at runtime against
 * xterm 6's internals. This test answers that question directly instead of trusting
 * the peer-dep metadata (which is simply stale -- addon-canvas will never be
 * updated again, since it no longer exists upstream to update).
 *
 * Verified independently (see PR discussion): the underscored private-service
 * surface `CanvasAddon.activate()` reaches into on `terminal._core` --
 * `coreService`, `optionsService`, `screenElement`, `linkifier`, `onWillOpen`,
 * `_bufferService`, `_renderService`, `_characterJoinerService`, `_charSizeService`,
 * `_coreBrowserService`, `_decorationService`, `_logService`, `_themeService` --
 * plus `renderService.setRenderer()`/`handleResize()`, all still exist textually in
 * the compiled `@xterm/xterm@6.0.0` bundle (confirmed via grep against
 * node_modules/.pnpm/@xterm+xterm@6.0.0/.../xterm.js). xterm.js 6.0.0's "Integrate
 * base/ platform from VS Code and adopt scroll bar" refactor (xtermjs/xterm.js#5096,
 * flagged as a breaking change upstream) was the change most likely to have broken
 * this wiring, and it did not rename or remove any of the fields addon-canvas needs.
 *
 * jsdom has no real 2D canvas context (`HTMLCanvasElement.prototype.getContext('2d')`
 * is "not implemented" -- the same limitation the codebase already works around for
 * `@xterm/addon-serialize` in jest.setup.js), so this test installs a minimal fake
 * 2D context covering every method CanvasAddon's render layers call (confirmed via
 * `grep -oh "_ctx\.\w\+" src/*.ts` against the unpacked addon-canvas 0.7.0 tarball).
 * This does not validate pixel-perfect rendering (impossible in jsdom regardless of
 * xterm version -- see project_plans/terminal-resize-fit-loop/research/pitfalls.md
 * §5, "Manual/E2E verification remains the only way to observe the real WebGL/Canvas
 * path"). What it DOES validate: that `CanvasAddon.activate()` completes without
 * throwing against the real xterm 6 Terminal, and that the terminal remains usable
 * (resize/write/dispose don't throw) afterward -- i.e., the actual API-compatibility
 * question this investigation set out to answer. If a *future* `@xterm/xterm` bump
 * ever does break this wiring, this test will fail loudly instead of silently
 * shipping a dead fallback behind a green peer-dep warning.
 */
import { Terminal } from "@xterm/xterm";
import { CanvasAddon } from "@xterm/addon-canvas";

function installFakeCanvas2dContext(): () => void {
  const original = HTMLCanvasElement.prototype.getContext;
  const fakeCtx = {
    save: jest.fn(),
    restore: jest.fn(),
    beginPath: jest.fn(),
    closePath: jest.fn(),
    clip: jest.fn(),
    rect: jest.fn(),
    fillRect: jest.fn(),
    strokeRect: jest.fn(),
    clearRect: jest.fn(),
    fillText: jest.fn(),
    measureText: jest.fn(() => ({ width: 9 }) as TextMetrics),
    moveTo: jest.fn(),
    lineTo: jest.fn(),
    bezierCurveTo: jest.fn(),
    stroke: jest.fn(),
    setLineDash: jest.fn(),
    drawImage: jest.fn(),
    getImageData: jest.fn(() => ({ data: new Uint8ClampedArray(4) }) as unknown as ImageData),
    putImageData: jest.fn(),
    createImageData: jest.fn(() => ({ data: new Uint8ClampedArray(4) }) as unknown as ImageData),
    fillStyle: "",
    strokeStyle: "",
    lineWidth: 1,
    font: "",
    textBaseline: "alphabetic" as CanvasTextBaseline,
  };
  (HTMLCanvasElement.prototype as any).getContext = jest.fn(() => fakeCtx);
  return () => {
    HTMLCanvasElement.prototype.getContext = original;
  };
}

describe("CanvasAddon vs xterm 6 runtime compatibility (ground-truth probe)", () => {
  let restoreGetContext: () => void;

  beforeEach(() => {
    restoreGetContext = installFakeCanvas2dContext();
    Object.defineProperty(window, "devicePixelRatio", { value: 1, configurable: true });
  });

  afterEach(() => {
    restoreGetContext();
  });

  it("CanvasAddon_should_ActivateAndStayUsable_When_LoadedAgainstRealXterm6Terminal", () => {
    const container = document.createElement("div");
    Object.defineProperty(container, "clientWidth", { value: 800, configurable: true });
    Object.defineProperty(container, "clientHeight", { value: 400, configurable: true });
    document.body.appendChild(container);

    const terminal = new Terminal();
    terminal.open(container);

    const addon = new CanvasAddon();
    // The real assertion: loading (activating) the addon against a real xterm 6
    // Terminal must not throw. A stale-peer-dep breakage would surface here as
    // the private services CanvasAddon.activate() destructures being undefined
    // (throwIfFalsy-style guards inside addon-canvas) or as a TypeError calling a
    // renamed/removed method.
    expect(() => terminal.loadAddon(addon)).not.toThrow();

    // A real incompatibility could also surface later (e.g. resize/write throwing
    // once the swapped-in renderer is actually exercised) rather than at
    // construction/activation time -- exercise both before disposing.
    expect(() => terminal.resize(100, 30)).not.toThrow();
    expect(() => terminal.write("hello")).not.toThrow();
    expect(() => addon.dispose()).not.toThrow();

    terminal.dispose();
  });
});
