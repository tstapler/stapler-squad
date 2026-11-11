/** Fake terminal shared by the xterm and ghostty-web module mocks (see mobileInputTestSetup.ts). */
/** A terminal that, like the real ones, owns a hidden helper textarea whose keydowns become `onData`. */
export const fakeTerminalSource = () => {
  const noop = () => ({ dispose: () => undefined });
  class FakeTerminal {
    static instances: FakeTerminal[] = [];
    static received: string[] = [];
    unicode = { activeVersion: "6" };
    cols = 80;
    rows = 24;
    options: Record<string, unknown>;
    modes = { bracketedPasteMode: false };
    buffer = { active: { length: 0, viewportY: 0 } };
    _core = { _renderService: { dimensions: undefined } };
    element?: HTMLElement;
    textarea?: HTMLTextAreaElement;
    opened?: HTMLElement;
    private listeners: Array<(data: string) => void> = [];
    constructor(options?: Record<string, unknown>) {
      this.options = options ?? {};
      FakeTerminal.instances.push(this);
    }
    open(container: HTMLElement) {
      this.opened = container;
      this.element = document.createElement("div");
      this.textarea = document.createElement("textarea");
      this.element.appendChild(this.textarea);
      container.appendChild(this.element);
      for (const type of ["beforeinput", "keydown", "input", "compositionstart", "compositionupdate", "compositionend"]) {
        container.addEventListener(type, (e) => {
          FakeTerminal.received.push(type);
          if (type === "beforeinput") e.preventDefault(); // ghostty-web's blanket handler
        });
      }
      this.textarea.addEventListener("keydown", (e) => {
        if (e.key.length === 1) this.listeners.forEach((l) => l(e.key));
      });
    }
    onData(cb: (data: string) => void) {
      this.listeners.push(cb);
      return { dispose: () => undefined };
    }
    onSelectionChange = noop;
    onResize = noop;
    onScroll = noop;
    loadAddon = () => undefined;
    hasBracketedPaste = () => false;
    hasMouseTracking = () => false;
    attachCustomWheelEventHandler = () => undefined;
    attachCustomKeyEventHandler = () => undefined;
    getSelection = () => "";
    focus = () => this.textarea?.focus();
    refresh = () => undefined;
    write = () => undefined;
    writeln = () => undefined;
    clear = () => undefined;
    resize = () => undefined;
    dispose = () => undefined;
  }
  return FakeTerminal;
};

