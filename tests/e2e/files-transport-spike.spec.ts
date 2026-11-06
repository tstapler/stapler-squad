// @feature files:tabs
// Task 0.1.8b (ADR-036 S-T5): does a save-shaped unary call queue behind a held search stream plus prefetching tabs?
// Default target is a local HTTP/1.1 node server that reproduces the browser's per-origin connection pool; set
// SPIKE_STREAM_PATH and SPIKE_UNARY_PATH to run against the isolated stapler-squad server's throwaway endpoints instead
// (that measured run is PENDING until the Go lane's handlers exist). Numbers are printed, never asserted.
import { test, expect, type Page } from "@playwright/test";
import * as http from "http";
import type { AddressInfo } from "net";

const PREFETCH_LEVELS = [0, 2, 4, 8];
const SAVE_CALLS = 20;
const QUEUED_MS = 50;

function startLocalServer(): Promise<http.Server> {
  const payload = "x".repeat(128 * 1024);
  const server = http.createServer((req, res) => {
    if (req.url?.startsWith("/stream")) {
      res.writeHead(200, { "content-type": "text/plain" });
      const timer = setInterval(() => res.write("tick\n"), 100);
      req.on("close", () => clearInterval(timer));
    } else if (req.url?.startsWith("/unary")) {
      setTimeout(() => res.writeHead(200, { "content-type": "text/plain" }).end(payload), 30);
    } else {
      res.writeHead(200, { "content-type": "text/html" }).end("<!doctype html><title>pool</title>");
    }
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve(server)));
}

async function holdStream(page: Page, path: string): Promise<void> {
  await page.evaluate(async (p) => {
    const ctrl = new AbortController();
    (window as unknown as { __abort: () => void }).__abort = () => ctrl.abort();
    const res = await fetch(p, { signal: ctrl.signal });
    const reader = res.body!.getReader();
    await reader.read();
    void (async () => {
      try {
        for (;;) if ((await reader.read()).done) break;
      } catch {
        /* aborted */
      }
    })();
  }, path);
}

async function prefetchFor(page: Page, path: string, inFlight: number, ms: number): Promise<void> {
  await page.evaluate(
    async ([p, n, d]) => {
      const end = performance.now() + (d as number);
      const lane = async () => {
        while (performance.now() < end) await (await fetch(`${p}?prefetch=${Math.random()}`)).arrayBuffer();
      };
      await Promise.all(Array.from({ length: n as number }, lane));
    },
    [path, inFlight, ms] as const
  );
}

async function saveShaped(page: Page, path: string, calls: number): Promise<{ latencies: number[]; queued: number }> {
  return page.evaluate(
    async ([p, n, q]) => {
      const latencies: number[] = [];
      let queued = 0;
      for (let i = 0; i < (n as number); i++) {
        const url = `${p}?save=${i}-${Math.random()}`;
        const t0 = performance.now();
        await (await fetch(url, { method: "POST", body: "{}" })).arrayBuffer();
        latencies.push(performance.now() - t0);
        const entry = performance.getEntriesByName(new URL(url, location.href).href).pop() as PerformanceResourceTiming | undefined;
        if (entry && entry.requestStart - entry.startTime > (q as number)) queued++;
      }
      return { latencies, queued };
    },
    [path, calls, QUEUED_MS] as const
  );
}

const pct = (v: number[], p: number) => [...v].sort((a, b) => a - b)[Math.min(v.length - 1, Math.ceil((p / 100) * v.length) - 1)];

test.describe("S-T5 connection-pool contention", () => {
  test("poolBudget_should_RecordSaveLatencyUnderStreamAndPrefetchingTabs", async ({ browser, browserName }) => {
    const streamPath = process.env.SPIKE_STREAM_PATH ?? "/stream";
    const unaryPath = process.env.SPIKE_UNARY_PATH ?? "/unary";
    const local = !process.env.SPIKE_STREAM_PATH;
    const server = local ? await startLocalServer() : null;
    const origin = server ? `http://127.0.0.1:${(server.address() as AddressInfo).port}` : (test.info().project.use.baseURL ?? "http://localhost:8544");

    const context = await browser.newContext();
    const [streamTab, prefetchA, prefetchB, saveTab] = await Promise.all([0, 1, 2, 3].map(() => context.newPage()));
    await Promise.all([streamTab, prefetchA, prefetchB, saveTab].map((p) => p.goto(origin + "/")));
    await holdStream(streamTab, streamPath);

    const rows: string[] = [];
    for (const level of PREFETCH_LEVELS) {
      const perTab = Math.ceil(level / 2);
      const load = level === 0 ? Promise.resolve() : Promise.all([prefetchFor(prefetchA, unaryPath, perTab, 4000), prefetchFor(prefetchB, unaryPath, level - perTab, 4000)]).then(() => undefined);
      const { latencies, queued } = await saveShaped(saveTab, unaryPath, SAVE_CALLS);
      await load;
      rows.push(`prefetch=${level} save p50=${Math.round(pct(latencies, 50))}ms p95=${Math.round(pct(latencies, 95))}ms queued=${queued}/${SAVE_CALLS}`);
      expect(latencies).toHaveLength(SAVE_CALLS);
    }
    console.log(`SPIKE 0.1.8b ${browserName} target=${local ? "local-node-http1" : origin} ${rows.join(" | ")}`);
    await streamTab.evaluate(() => (window as unknown as { __abort: () => void }).__abort());
    await context.close();
    server?.closeAllConnections();
    server?.close();
  });
});
