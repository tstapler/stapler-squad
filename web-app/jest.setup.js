// jest.setup.js — runs after the test framework is installed in the environment.
//
// Polyfills TextEncoder/TextDecoder which are missing from older jsdom versions
// but required by @bufbuild/protobuf.
const { TextEncoder, TextDecoder } = require("util");
Object.assign(globalThis, { TextEncoder, TextDecoder });

// jest-dom matchers (toBeInTheDocument, etc.)
require("@testing-library/jest-dom");

// ResizeObserver is not available in jsdom. Provide a stub that fires the callback
// synchronously with the element's current getBoundingClientRect() when observe() is
// called. This mirrors real-browser behavior (observer fires soon after observe())
// and lets tests that mock getBoundingClientRect() control the reported size.
// Tests that need a fully controllable ResizeObserver can override with:
//   Object.defineProperty(global, 'ResizeObserver', { writable: true, value: ... })
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
