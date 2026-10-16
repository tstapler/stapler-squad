import {
  DEBUG_FLAG_KEY,
  MAX_DEBUG_ENTRIES,
  MISROUTE_OVERRIDE_WINDOW_MS,
  MISROUTE_TOOLBAR_WINDOW_MS,
  MobileDebugLog,
  type DebugEntry,
  type TermDebugApi,
} from "../mobileDebug";

function makeLog(flag: boolean) {
  const clock = { now: 1_000 };
  const sink = jest.fn();
  const target: { __termDebug?: TermDebugApi } = {};
  const flagRef = { value: flag };
  const log = new MobileDebugLog({
    now: () => clock.now,
    isEnabled: () => flagRef.value,
    sink,
    target,
  });
  return { log, clock, sink, target, flagRef };
}

describe("MobileDebugLog", () => {
  it("log_should_BufferEntriesAndEmitConsoleDebug_When_FlagTrue", () => {
    const { log, sink } = makeLog(true);

    log.log("scroll", { lines: 3 });

    const entries = JSON.parse(log.dump()) as DebugEntry[];
    expect(entries).toEqual([{ t: 1_000, type: "scroll", data: { lines: 3 } }]);
    expect(sink).toHaveBeenCalledTimes(1);
  });

  it("dump_should_Return500NewestEntries_When_600Logged", () => {
    const { log } = makeLog(true);

    for (let i = 0; i < 600; i++) log.log("scroll", { i });

    const entries = JSON.parse(log.dump()) as DebugEntry[];
    expect(entries).toHaveLength(MAX_DEBUG_ENTRIES);
    expect(entries[0].data).toEqual({ i: 100 });
    expect(entries[499].data).toEqual({ i: 599 });
  });

  it("log_should_AddNothingAndNotCallConsoleDebug_When_FlagUnsetAnd1000Calls", () => {
    const { log, sink } = makeLog(false);
    const consoleDebug = jest.spyOn(console, "debug").mockImplementation(() => {});

    for (let i = 0; i < 1000; i++) log.log("scroll", { i });

    expect(JSON.parse(log.dump())).toHaveLength(0);
    expect(sink).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
    consoleDebug.mockRestore();
  });

  it("stats_should_CountOverrideChangeAndMisroute_When_OverrideChangedWithin10sOfAutoDrag", () => {
    const { log, clock } = makeLog(true);

    log.routeDecision({ target: "tui", source: "auto", unverified: true });
    clock.now += MISROUTE_OVERRIDE_WINDOW_MS;
    log.overrideChange("auto", "local");
    clock.now += 1;
    log.overrideChange("local", "auto"); // 10.001 s after the drag: outside the window

    expect(log.stats()).toEqual({
      routeDecisions: 1,
      overrideChanges: 2,
      misroutes: { overrideAfterDrag: 1, toolbarKeyAfterDrag: 0 },
    });
    const types = (JSON.parse(log.dump()) as DebugEntry[]).map((e) => e.type);
    expect(types.filter((t) => t === "misroute-proxy")).toHaveLength(1);
  });

  it("stats_should_CountToolbarKeyAfterDrag_When_Within5s", () => {
    const { log, clock } = makeLog(true);

    log.routeDecision({ target: "local", source: "auto", unverified: false });
    clock.now += MISROUTE_TOOLBAR_WINDOW_MS;
    log.toolbarKey("PageUp");
    clock.now += 1;
    log.toolbarKey("PageDown"); // outside the 5 s window

    expect(log.stats()?.misroutes).toEqual({ overrideAfterDrag: 0, toolbarKeyAfterDrag: 1 });
    const proxies = (JSON.parse(log.dump()) as DebugEntry[]).filter((e) => e.type === "misroute-proxy");
    expect(proxies).toEqual([
      { t: 6_000, type: "misroute-proxy", data: { kind: "toolbar-key-after-drag" } },
    ]);
  });

  it("stats_should_NotCountMisroute_When_RouteWasNotAuto", () => {
    const { log } = makeLog(true);

    log.routeDecision({ target: "tui", source: "override", unverified: false });
    log.overrideChange("tui", "auto");
    log.toolbarKey("PageUp");

    expect(log.stats()?.misroutes).toEqual({ overrideAfterDrag: 0, toolbarKeyAfterDrag: 0 });
  });

  it("stats_should_BeUnavailableAndZeroOverhead_When_FlagUnset", () => {
    const { log, sink, target } = makeLog(false);

    log.routeDecision({ target: "tui", source: "auto", unverified: true });
    log.overrideChange("auto", "local");
    log.toolbarKey("PageUp");

    expect(log.stats()).toBeNull();
    expect(JSON.parse(log.dump())).toHaveLength(0);
    expect(sink).not.toHaveBeenCalled();
    log.install();
    expect(target.__termDebug?.stats()).toBeNull();
  });

  it("install_should_RegisterTermDebugReadingFlagLazily", () => {
    const { log, target, flagRef } = makeLog(false);
    log.install();

    log.log("scroll", {});
    expect(JSON.parse(target.__termDebug!.dump())).toHaveLength(0);

    flagRef.value = true;
    log.log("scroll", {});
    expect(JSON.parse(target.__termDebug!.dump())).toHaveLength(1);
  });

  it("flag_should_ReadLocalStorageKey_When_NoIsEnabledInjected", () => {
    const log = new MobileDebugLog({ sink: jest.fn(), target: {} });

    localStorage.removeItem(DEBUG_FLAG_KEY);
    log.log("scroll", {});
    expect(JSON.parse(log.dump())).toHaveLength(0);

    localStorage.setItem(DEBUG_FLAG_KEY, "true");
    log.log("scroll", {});
    expect(JSON.parse(log.dump())).toHaveLength(1);
    localStorage.removeItem(DEBUG_FLAG_KEY);
  });
});
