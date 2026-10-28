import { Page, Locator, CDPSession } from '@playwright/test';

export interface PerfGauge {
  peakInFlight: number;
  queuedOver1s: number;
  queuedOver1sCritical: number;
  queuedOver1sDeferred: number;
}

/** Page helper for the frontend-responsiveness perf spec; end conditions are `ssq:*` User Timing measures, never `networkidle`. */
export class PerfPage {
  constructor(readonly page: Page) {}

  get windowTabs(): Locator {
    return this.page.locator('[data-testid^="window-tab-"]:not([data-testid^="window-tab-close-"])');
  }

  /** Counters/observers every scenario reads; installed before any app script. */
  async installObservers(): Promise<void> {
    await this.page.addInitScript(() => {
      const w = window as unknown as { __lt: number[]; __xt: number };
      w.__lt = [];
      w.__xt = 0;
      new PerformanceObserver((l) => l.getEntries().forEach((e) => w.__lt.push(e.duration))).observe({ entryTypes: ['longtask'] });
      new MutationObserver((m) =>
        m.forEach((x) =>
          x.addedNodes.forEach((n) => {
            const el = n as Element;
            if (el.nodeType === 1 && (el.matches?.('.xterm') || el.querySelector?.('.xterm'))) w.__xt++;
          })
        )
      ).observe(document, { childList: true, subtree: true });
      // SM6: input-to-next-paint (Event Timing, INP definition) for pointer/key interactions.
      (window as unknown as { __ev: number[] }).__ev = [];
      try {
        new PerformanceObserver((l) =>
          l.getEntries().forEach((e) => {
            if (e.name === 'click' || e.name === 'keydown') (window as unknown as { __ev: number[] }).__ev.push(e.duration);
          })
        ).observe({ type: 'event', durationThreshold: 16, buffered: false } as PerformanceObserverInit);
      } catch {
        // Event Timing unsupported: SM6 reported as null.
      }
    });
  }

  async resetCounters(): Promise<void> {
    await this.page.evaluate(() => {
      const w = window as unknown as { __lt: number[]; __xt: number; __ev: number[] };
      w.__lt = [];
      w.__xt = 0;
      w.__ev = [];
      performance.clearMeasures('ssq:window-switch');
      performance.clearMeasures('ssq:route-change');
      const p = (window as unknown as { __ssqPerf?: { inFlight: number; peakInFlight: number; queuedOver1s: number; queuedOver1sByClass: { critical: number; deferred: number } } }).__ssqPerf;
      if (p) {
        p.peakInFlight = p.inFlight;
        p.queuedOver1s = 0;
        p.queuedOver1sByClass = { critical: 0, deferred: 0 };
      }
    });
  }

  async readCounters(): Promise<{ longTasks: number[]; xtermMounts: number; eventDurations: number[]; gauge: PerfGauge | null }> {
    return this.page.evaluate(() => {
      const w = window as unknown as { __lt: number[]; __xt: number; __ev: number[] };
      const p = (window as unknown as { __ssqPerf?: { inFlight: number; peakInFlight: number; queuedOver1s: number; queuedOver1sByClass: { critical: number; deferred: number } } }).__ssqPerf;
      return {
        longTasks: [...w.__lt],
        xtermMounts: w.__xt,
        eventDurations: [...w.__ev],
        gauge: p
          ? {
              peakInFlight: p.peakInFlight,
              queuedOver1s: p.queuedOver1s,
              queuedOver1sCritical: p.queuedOver1sByClass.critical,
              queuedOver1sDeferred: p.queuedOver1sByClass.deferred,
            }
          : null,
      };
    });
  }

  async measureCount(name: string): Promise<number> {
    return this.page.evaluate((n) => performance.getEntriesByName(n).length, name);
  }

  /** Resolves when `performance.getEntriesByName(name)` grows past `before`. */
  async waitForMeasureGrowth(name: string, before: number, timeout = 240000): Promise<void> {
    await this.page.waitForFunction(([n, b]) => performance.getEntriesByName(n as string).length > (b as number), [name, before] as const, { timeout, polling: 'raf' });
  }

  async lastMeasureMs(name: string): Promise<number | null> {
    return this.page.evaluate((n) => {
      const e = performance.getEntriesByName(n);
      return e.length ? e[e.length - 1].duration : null;
    }, name);
  }

  /** Clicks window tab `index` and waits for the `ssq:window-switch` measure; returns its duration. */
  async switchWindow(index: number): Promise<number | null> {
    const before = await this.measureCount('ssq:window-switch');
    // Records whether the clicked tab shows aria-selected on the first frame after the click (SM6).
    await this.page.evaluate((i) => {
      const w = window as unknown as { __hl: boolean };
      w.__hl = false;
      const tabs = document.querySelectorAll('[data-testid^="window-tab-"]:not([data-testid^="window-tab-close-"])');
      document.addEventListener('click', () => requestAnimationFrame(() => { w.__hl = tabs[i]?.getAttribute('aria-selected') === 'true'; }), { capture: true, once: true });
    }, index);
    await this.windowTabs.nth(index).click({ force: true });
    await this.waitForMeasureGrowth('ssq:window-switch', before);
    return this.lastMeasureMs('ssq:window-switch');
  }

  async lastHighlightFirstFrame(): Promise<boolean> {
    return this.page.evaluate(() => (window as unknown as { __hl: boolean }).__hl);
  }

  async dismissOverlays(): Promise<void> {
    for (let i = 0; i < 3; i++) await this.page.keyboard.press('Escape');
    await this.page.evaluate(() => document.querySelectorAll('[data-state=open][aria-hidden=true]').forEach((e) => e.remove()));
  }

  async waitForShell(): Promise<void> {
    await this.windowTabs.first().waitFor({ timeout: 90000 });
  }

  /** SPA nav through an in-app anchor (mirrors prof.cjs), ending on `ssq:route-change`. */
  async spaNavigate(href: '/' | '/backlog'): Promise<void> {
    const before = await this.measureCount('ssq:route-change');
    const name = href === '/' ? 'Sessions' : 'Backlog';
    await this.page.getByRole('link', { name, exact: true }).first().click({ force: true });
    await this.waitForMeasureGrowth('ssq:route-change', before);
  }

  async setCpuThrottle(cdp: CDPSession, rate: 1 | 4): Promise<void> {
    await cdp.send('Emulation.setCPUThrottlingRate', { rate });
  }

  async layoutCount(cdp: CDPSession): Promise<number> {
    await cdp.send('Performance.enable');
    const { metrics } = await cdp.send('Performance.getMetrics');
    return metrics.find((m) => m.name === 'LayoutCount')?.value ?? 0;
  }
}
