// Script tests run under node:test (`bun run test:codemods`), not Jest: ESM .mjs needs
// --experimental-vm-modules under Jest. The `build` Jest project collects only build/**.
import assert from "node:assert/strict";
import { describe, it } from "node:test";

describe("script test runner", () => {
  it("collects and runs .mjs tests under Node", () => {
    assert.equal(typeof Bun, "undefined");
  });
});
