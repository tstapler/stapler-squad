import { runInBun } from "./helpers/run-in-bun";

interface DevFetch {
  hostname: string;
  deep: { status: number; type: string; hasHtml: boolean };
  sw: { status: number; body: string };
  icon: { status: number; body: string };
  sse: { type: string; got: boolean };
  second: { isPortInUse: boolean; message: string; addrInUse: boolean } | null;
  clientLogs: { status: number; echoed: string | undefined };
  clientLogsNoApi: number;
  shell: string;
  envDefault: { port: number; hmr: boolean };
  envHmrOff: { port: number; hmr: boolean };
  envBad: string;
}

describe("dev server (under Bun)", () => {
  const r = runInBun("dev-fetch.bun.ts");
  if (r.status !== 0) throw new Error(`dev-fetch.bun.ts failed: ${r.stderr}`);
  const out = r.json as DevFetch;

  it("binds to localhost only", () => {
    expect(out.hostname).toBe("localhost");
  });

  it("serves the shell for a deep link with a query string", () => {
    expect(out.deep).toMatchObject({ status: 200, hasHtml: true });
    expect(out.deep.type).toContain("text/html");
  });

  it("serves public/ files at the root, nested ones included", () => {
    expect(out.sw).toEqual({ status: 200, body: "// SW_MARKER\n" });
    expect(out.icon).toEqual({ status: 200, body: "<svg/>" });
  });

  it("streams a reload event to connected pages when the hub broadcasts", () => {
    expect(out.sse.type).toContain("text/event-stream");
    expect(out.sse.got).toBe(true);
  });

  it("fails with a named port and an ss hint, never a raw stack, when the port is taken", () => {
    expect(out.second?.isPortInUse).toBe(true);
    expect(out.second?.message).toMatch(/port \d+ is already in use/);
    expect(out.second?.message).toContain("ss -ltnp");
    expect(out.second?.message).not.toContain("    at ");
  });

  it("forwards /api/client-logs to NEXT_PUBLIC_API_URL when one is configured, and no-ops (204) otherwise", () => {
    expect(out.clientLogs).toEqual({ status: 207, echoed: "payload" });
    expect(out.clientLogsNoApi).toBe(204);
  });

  it("rewrites index.html into a dev shell: process shim, then theme-boot, then the app, paths relative to the shell", () => {
    expect(out.shell).not.toContain("<!--");
    expect(out.shell.indexOf("globalThis.process")).toBeLessThan(out.shell.indexOf("theme-boot.ts"));
    expect(out.shell.indexOf("theme-boot.ts")).toBeLessThan(out.shell.indexOf("main.tsx"));
    expect(out.shell).toContain('src="../../src/theme-boot.ts"');
    expect(out.shell).toContain('src="../../src/main.tsx"');
    expect(out.shell).toContain("EventSource");
  });

  it("parses PORT and WEB_DEV_HMR", () => {
    expect(out.envDefault).toEqual({ port: 3001, hmr: true });
    expect(out.envHmrOff).toEqual({ port: 62875, hmr: false });
    expect(out.envBad).toContain("PORT must be an integer");
  });
});
