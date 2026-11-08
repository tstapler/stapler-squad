import { attachRenderGate, pauseRenderLoop, resumeRenderLoop } from "../ghosttyRenderGate";

/** Mimics ghostty-web's startRenderLoop: render, then re-arm requestAnimationFrame while open. */
function makeLoopingTerminal(frames: Map<number, () => void>) {
  let nextId = 1;
  const term = {
    renders: 0,
    isOpen: true,
    isDisposed: false,
    animationFrameId: undefined as number | undefined,
    startRenderLoop() {
      const tick = () => {
        if (this.isDisposed || !this.isOpen) return;
        this.renders++;
        const id = nextId++;
        frames.set(id, tick);
        this.animationFrameId = id;
      };
      tick();
    },
  };
  return term;
}

function runFrame(frames: Map<number, () => void>) {
  const pending = [...frames.entries()];
  frames.clear();
  for (const [, cb] of pending) cb();
}

describe("ghosttyRenderGate", () => {
  let frames: Map<number, () => void>;
  beforeEach(() => {
    frames = new Map();
    jest.spyOn(globalThis, "cancelAnimationFrame").mockImplementation((id) => {
      frames.delete(id);
    });
  });
  afterEach(() => jest.restoreAllMocks());

  it("stops rendering after pause and repaints on resume", () => {
    const term = makeLoopingTerminal(frames);
    term.startRenderLoop();
    runFrame(frames);
    expect(term.renders).toBe(2);

    expect(pauseRenderLoop(term)).toBe(true);
    runFrame(frames);
    runFrame(frames);
    expect(term.renders).toBe(2);

    resumeRenderLoop(term);
    expect(term.renders).toBe(3);
    runFrame(frames);
    expect(term.renders).toBe(4);
  });

  it("does not start a second loop when resumed while already running", () => {
    const term = makeLoopingTerminal(frames);
    term.startRenderLoop();
    resumeRenderLoop(term);
    expect(frames.size).toBe(1);
  });

  describe("attachRenderGate", () => {
    let fire: (isIntersecting: boolean) => void;
    const disconnect = jest.fn();
    class FakeObserver {
      constructor(cb: IntersectionObserverCallback) {
        fire = (isIntersecting) => cb([{ isIntersecting } as IntersectionObserverEntry], this as unknown as IntersectionObserver);
      }
      observe() {}
      disconnect = disconnect;
    }
    const gate = (term: object, doc: Document) =>
      attachRenderGate(term, document.createElement("div"), doc, FakeObserver as unknown as typeof IntersectionObserver);

    it("pauses while off-screen and resumes when it intersects again", () => {
      const term = makeLoopingTerminal(frames);
      term.startRenderLoop();
      gate(term, document);

      fire(false);
      runFrame(frames);
      runFrame(frames);
      expect(term.renders).toBe(1);

      fire(true);
      expect(term.renders).toBe(2);
      runFrame(frames);
      expect(term.renders).toBe(3);
    });

    it("pauses while the document is hidden even if the terminal intersects", () => {
      const term = makeLoopingTerminal(frames);
      term.startRenderLoop();
      const doc = new EventTarget() as unknown as Document & { visibilityState: string };
      doc.visibilityState = "visible";
      gate(term, doc);

      doc.visibilityState = "hidden";
      doc.dispatchEvent(new Event("visibilitychange"));
      runFrame(frames);
      expect(term.renders).toBe(1);

      doc.visibilityState = "visible";
      doc.dispatchEvent(new Event("visibilitychange"));
      expect(term.renders).toBe(2);
    });

    it("no-ops without IntersectionObserver", () => {
      const term = makeLoopingTerminal(frames);
      term.startRenderLoop();
      const detach = attachRenderGate(term, document.createElement("div"), document, undefined);
      detach();
      runFrame(frames);
      expect(term.renders).toBe(2);
    });
  });
});
