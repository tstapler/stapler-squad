import * as React from "react";
import { renderHook, act } from "@testing-library/react";

type E2eWindow = Window & { __e2eTriggerInputDropped?: unknown; __E2E_HOOKS__?: boolean };
const target = window as E2eWindow;
const originalFetch = global.fetch;

/** Fresh module per case: the server's answer is cached for the page's lifetime. */
async function mountWith(fetchImpl: () => Promise<unknown>) {
  let useTrigger!: (report: (count: number) => void) => void;
  jest.isolateModules(() => {
    jest.doMock("react", () => React); // one React instance, shared with renderHook
    useTrigger = require("./useE2eInputDropTrigger").useE2eInputDropTrigger;
  });
  global.fetch = jest.fn(fetchImpl) as unknown as typeof fetch;
  const report = jest.fn();
  const view = renderHook(() => useTrigger(report));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  return { report, view };
}

const serverInfo = (body: object) => async () => ({ json: async () => body });

describe("useE2eInputDropTrigger", () => {
  afterEach(() => {
    global.fetch = originalFetch;
    delete target.__E2E_HOOKS__;
    delete target.__e2eTriggerInputDropped;
  });

  it("does not install on a production server, even if a page script asks for it", async () => {
    target.__E2E_HOOKS__ = true;
    await mountWith(serverInfo({ version: "1.0.0" }));
    expect(target.__e2eTriggerInputDropped).toBeUndefined();
  });

  it("does not install when server-info cannot be read", async () => {
    await mountWith(async () => {
      throw new Error("offline");
    });
    expect(target.__e2eTriggerInputDropped).toBeUndefined();
  });

  it("does not install when unmounted before the server answers", async () => {
    let answer!: (value: unknown) => void;
    const { view } = await mountWith(() => new Promise((resolve) => (answer = resolve)));
    view.unmount();
    await act(async () => {
      answer({ json: async () => ({ e2e_hooks: true }) });
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(target.__e2eTriggerInputDropped).toBeUndefined();
  });

  it("installs on the e2e server and removes itself on unmount", async () => {
    const { report, view } = await mountWith(serverInfo({ e2e_hooks: true }));
    expect(target.__e2eTriggerInputDropped).toBe(report);
    view.unmount();
    expect(target.__e2eTriggerInputDropped).toBeUndefined();
  });
});
