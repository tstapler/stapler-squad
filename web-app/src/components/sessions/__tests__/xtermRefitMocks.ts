/**
 * Shared xterm module doubles for XtermTerminal refit tests. Used from
 * jest.mock factories via require() so several test files can reuse one copy
 * (keeps the jscpd duplication gate quiet). Instances are recorded in `created`.
 */

interface Created {
  terminals: any[];
  fits: any[];
  webgls: any[];
  canvases: any[];
}

export const created: Created = { terminals: [], fits: [], webgls: [], canvases: [] };

export function resetCreated(): void {
  created.terminals.length = 0;
  created.fits.length = 0;
  created.webgls.length = 0;
  created.canvases.length = 0;
}

const noopDisposable = () => ({ dispose: jest.fn() });

export function xtermModule() {
  class Terminal {
    cols = 80;
    rows = 24;
    options: any;
    buffer = { active: { length: 0, viewportY: 0 } };
    _core = { _renderService: { dimensions: undefined } };
    refresh = jest.fn();
    clearTextureAtlas = jest.fn();
    loadAddon = jest.fn();
    open = jest.fn();
    onData = jest.fn(noopDisposable);
    onSelectionChange = jest.fn(noopDisposable);
    onResize = jest.fn(noopDisposable);
    getSelection = () => "";
    write = jest.fn();
    focus = jest.fn();
    clear = jest.fn();
    dispose = jest.fn();
    constructor(options?: any) {
      this.options = options ?? {};
      created.terminals.push(this);
    }
  }
  return { Terminal };
}

export function fitModule() {
  class FitAddon {
    fit = jest.fn();
    proposeDimensions = jest.fn((): { cols: number; rows: number } | undefined => undefined);
    constructor() {
      created.fits.push(this);
    }
  }
  return { FitAddon };
}

export function webglModule() {
  class WebglAddon {
    dispose = jest.fn();
    contextLossCb: (() => void) | null = null;
    onContextLoss = jest.fn((cb: () => void) => {
      this.contextLossCb = cb;
    });
    constructor() {
      created.webgls.push(this);
    }
  }
  return { WebglAddon };
}

export function canvasModule() {
  class CanvasAddon {
    dispose = jest.fn();
    constructor() {
      created.canvases.push(this);
    }
  }
  return { CanvasAddon };
}

export function searchModule() {
  return { SearchAddon: class { findNext = jest.fn(() => true); findPrevious = jest.fn(() => true); } };
}
