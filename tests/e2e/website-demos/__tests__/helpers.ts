// @feature website-demos
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

export function tmpDir(prefix = 'wd-'): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

export interface FakeOpts {
  /** Computed body background the fake page reports. */
  background?: string;
  domText?: string;
  url?: string;
}

/** Just enough Page/Context/Browser for recordClip: evaluate is routed on the function source. */
export function fakeBrowser(opts: FakeOpts = {}) {
  const calls = { contexts: 0 };
  const page: any = {
    url: () => opts.url ?? 'http://localhost/',
    evaluate: async (fn: (...a: unknown[]) => unknown) => {
      const src = String(fn);
      if (src.includes('backgroundColor')) return { stored: 'grimdark', background: opts.background ?? 'rgb(12, 10, 8)' };
      if (src.includes('fonts.ready')) return true;
      if (src.includes('innerText')) return opts.domText ?? 'Six agents on screen';
      return undefined;
    },
    locator: () => ({ count: async () => 0, first: () => ({ waitFor: async () => undefined }) }),
    screenshot: async ({ path: p }: { path: string }) => {
      fs.mkdirSync(path.dirname(p), { recursive: true });
      fs.writeFileSync(p, 'png-bytes');
    },
    emulateMedia: async () => undefined,
    goto: async () => undefined,
    video: () => ({
      saveAs: async (p: string) => {
        fs.mkdirSync(path.dirname(p), { recursive: true });
        fs.writeFileSync(p, 'webm-bytes');
      },
    }),
  };
  const context: any = {
    clock: { setFixedTime: async () => undefined },
    addInitScript: async () => undefined,
    newPage: async () => page,
    close: async () => undefined,
  };
  const browser: any = {
    newContext: async () => {
      calls.contexts++;
      return context;
    },
  };
  return { browser, page, calls };
}
