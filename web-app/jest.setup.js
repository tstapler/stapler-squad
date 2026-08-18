// jest.setup.js — runs after the test framework is installed in the environment.
//
// Polyfills TextEncoder/TextDecoder which are missing from older jsdom versions
// but required by @bufbuild/protobuf.
const { TextEncoder, TextDecoder } = require("util");
Object.assign(globalThis, { TextEncoder, TextDecoder });

// ReadableStream/TransformStream/CompressionStream/DecompressionStream are not available in
// jsdom. node:stream/web provides spec-compliant implementations of all four — used here so
// websocket-transport.ts's gzip decompression path (Task 5.1.1.0, terminal-resync-reliability
// Epic 5.1) can run under Jest the same way it runs in a real browser.
const {
  ReadableStream,
  TransformStream,
  CompressionStream,
  DecompressionStream,
} = require("node:stream/web");
if (typeof globalThis.ReadableStream === "undefined") {
  globalThis.ReadableStream = ReadableStream;
}
if (typeof globalThis.TransformStream === "undefined") {
  globalThis.TransformStream = TransformStream;
}
if (typeof globalThis.CompressionStream === "undefined") {
  globalThis.CompressionStream = CompressionStream;
}
if (typeof globalThis.DecompressionStream === "undefined") {
  globalThis.DecompressionStream = DecompressionStream;
}

// jest-dom matchers (toBeInTheDocument, etc.)
require("@testing-library/jest-dom");

// ResizeObserver is not available in jsdom. Provide a stub that fires the callback
// synchronously with the element's current getBoundingClientRect() when observe() is
// called. This mirrors real-browser behavior (observer fires soon after observe())
// and lets tests that mock getBoundingClientRect() control the reported size.
// Tests that need a fully controllable ResizeObserver can override with:
//   Object.defineProperty(global, 'ResizeObserver', { writable: true, value: ... })
// XtermTerminal.test.tsx does exactly this (installCapturingResizeObserver) to get a
// deliberately inert, manually-driven ResizeObserver for its dead-band sampler tests
// (ADR-002) — the auto-firing behavior here would otherwise start the sampler
// immediately on every mount, before the fake-timer-driven test bodies are ready.
// BroadcastChannel is not available in jsdom. Provide a minimal stub so that
// components using createNotificationSyncChannel() don't throw in tests.
// The stub is fire-and-forget; tests that need to assert cross-tab messages
// should replace this with a mock via jest.spyOn or Object.defineProperty.
if (typeof globalThis.BroadcastChannel === "undefined") {
  globalThis.BroadcastChannel = class BroadcastChannel {
    constructor(_name) {}
    postMessage(_data) {}
    addEventListener(_type, _listener) {}
    removeEventListener(_type, _listener) {}
    close() {}
  };
}

if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    constructor(callback) {
      this._callback = callback;
    }
    observe(el) {
      const rect = el.getBoundingClientRect();
      const entry = {
        target: el,
        contentRect: {
          width: rect.width,
          height: rect.height,
          top: 0,
          left: 0,
          bottom: rect.height,
          right: rect.width,
          x: 0,
          y: 0,
        },
      };
      this._callback([entry], this);
    }
    unobserve() {}
    disconnect() {}
  };
}

// window.matchMedia is not available in jsdom. Provide a minimal stub.
// Tests that need specific match values can override via jest.spyOn or Object.defineProperty.
if (typeof window !== "undefined" && typeof window.matchMedia === "undefined") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: jest.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

// next/navigation is not available in the jsdom test environment.
// Tests that import components using useRouter or usePathname from next/navigation
// will fail without this stub. Individual tests can override via jest.mock().
jest.mock("next/navigation", () => ({
  useRouter: jest.fn(() => ({ push: jest.fn(), replace: jest.fn(), back: jest.fn(), forward: jest.fn() })),
  usePathname: jest.fn(() => "/"),
  useSearchParams: jest.fn(() => new URLSearchParams()),
  useParams: jest.fn(() => ({})),
}), { virtual: true });

// @xterm/addon-serialize calls HTMLCanvasElement.prototype.getContext at module load time,
// which jsdom does not implement. Stub it globally to prevent the canvas error and keep
// xterm tests isolated. Individual test files that need SerializeAddon behavior can override.
jest.mock("@xterm/addon-serialize", () => ({
  SerializeAddon: class SerializeAddon {
    serialize() { return ""; }
    serializeRows() { return ""; }
    serializeRowRange() { return ""; }
    activate() {}
    dispose() {}
  },
}));

// useSlashCommands makes a ConnectRPC network call (via fetch) on mount.
// Stub it globally to prevent unexpected fetch calls in tests that assert
// exact fetch call counts (e.g. image-upload tests).
jest.mock("@/lib/hooks/useSlashCommands", () => ({
  useSlashCommands: () => ({ commands: [], isLoading: false }),
}));

// useAvailablePrograms fetches /api/server-info on mount. In test environments
// this extra fetch call interferes with tests that assert exact fetch call counts
// (e.g. image-upload tests). Stub the hook globally to return the static PROGRAMS
// list without performing any network requests. Tests that specifically need to
// exercise program-loading behavior should override this mock locally.
jest.mock("@/lib/hooks/useAvailablePrograms", () => ({
  useAvailablePrograms: () => {
    const { PROGRAMS } = require("@/lib/constants/programs");
    return PROGRAMS;
  },
}));
