import { readdirSync, readFileSync, statSync } from "fs";
import { join, relative } from "path";
import { isTouchPrimary, shouldAttachMobileInput } from "./platform";

const ANDROID_UA = "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36";
const IPHONE_UA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Safari/604.1";
const IPADOS_UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15";
const WIN_UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120 Safari/537.36";

interface Env {
  ua: string;
  coarse: boolean;
  hover: boolean;
  uaDataPlatform?: string | null | "absent";
  standalone?: boolean;
}

function setEnv(e: Env) {
  Object.defineProperty(navigator, "userAgent", { value: e.ua, configurable: true });
  Object.defineProperty(navigator, "userAgentData", {
    value: e.uaDataPlatform === "absent" || e.uaDataPlatform === undefined ? undefined : { platform: e.uaDataPlatform },
    configurable: true,
  });
  window.matchMedia = ((q: string) => ({
    matches:
      q === "(pointer: coarse)" ? e.coarse : q === "(hover: hover)" ? e.hover : q === "(display-mode: standalone)" ? !!e.standalone : false,
  })) as unknown as typeof window.matchMedia;
}

const rows: Array<[string, Env, boolean, boolean]> = [
  // name, env, shouldAttach, isTouchPrimary
  ["android coarse no-hover (UA-CH)", { ua: ANDROID_UA, coarse: true, hover: false, uaDataPlatform: "Android" }, true, true],
  ["android UA-CH platform null", { ua: ANDROID_UA, coarse: true, hover: false, uaDataPlatform: null }, true, true],
  ["android UA-CH platform empty", { ua: ANDROID_UA, coarse: true, hover: false, uaDataPlatform: "" }, true, true],
  ["android plain HTTP (no userAgentData)", { ua: ANDROID_UA, coarse: true, hover: false }, true, true],
  ["android standalone PWA", { ua: ANDROID_UA, coarse: true, hover: false, standalone: true }, true, true],
  ["iPhone", { ua: IPHONE_UA, coarse: true, hover: false }, false, true],
  ["iPadOS desktop UA with touch", { ua: IPADOS_UA, coarse: true, hover: false }, false, true],
  ["Windows touch laptop (hover)", { ua: WIN_UA, coarse: true, hover: true }, false, true],
  ["android with mouse (fine pointer)", { ua: ANDROID_UA, coarse: false, hover: true, uaDataPlatform: "Android" }, false, false],
  ["desktop", { ua: WIN_UA, coarse: false, hover: true }, false, false],
];

describe("platform predicates", () => {
  it.each(rows)("shouldAttachMobileInput %s", (_n, env, attach) => {
    setEnv(env);
    expect(shouldAttachMobileInput()).toBe(attach);
  });

  it.each(rows)("isTouchPrimary %s", (_n, env, _a, touch) => {
    setEnv(env);
    expect(isTouchPrimary()).toBe(touch);
  });
});

describe("pointer-coarse matchMedia scope", () => {
  const SRC = join(__dirname, "../../..");
  const PINNED = /matchMedia\(\s*['"]\(pointer:\s*coarse\)['"]\s*\)/;

  function walk(dir: string, out: string[] = []): string[] {
    for (const name of readdirSync(dir)) {
      const p = join(dir, name);
      if (statSync(p).isDirectory()) walk(p, out);
      else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name) && !p.includes("__tests__")) out.push(p);
    }
    return out;
  }

  const scope = [
    ...walk(join(SRC, "lib/terminal")),
    ...["XtermTerminal", "GhosttyTerminal", "TerminalOutput"].map((n) => join(SRC, `components/sessions/${n}.tsx`)),
  ];

  it("terminalScope_should_ContainPointerCoarseMatchMediaOnlyInPlatformTs_When_Scanned", () => {
    const hits = scope.filter((f) => PINNED.test(readFileSync(f, "utf8"))).map((f) => relative(SRC, f));
    // platform.ts builds the query via a helper, so it spells the string literal rather than a matchMedia( call.
    expect(hits).toEqual([]);
    expect(readFileSync(join(SRC, "lib/terminal/mobileInput/platform.ts"), "utf8")).toContain('"(pointer: coarse)"');
  });

  it("terminalScope_should_ImportIsTouchPrimaryFromPlatform_When_XtermTerminalScanned", () => {
    const src = readFileSync(join(SRC, "components/sessions/XtermTerminal.tsx"), "utf8");
    expect(src).toMatch(/isTouchPrimary(\s+as\s+\w+)?\s*}\s*from\s*"@\/lib\/terminal\/mobileInput\/platform"/);
  });

  it("pinnedRegex_should_MatchExactQueryOnly_When_NegativeFixturesScanned", () => {
    expect(PINNED.test(`matchMedia('(pointer: coarse)')`)).toBe(true);
    expect(PINNED.test(`matchMedia("(any-pointer: coarse)")`)).toBe(false);
    expect(PINNED.test(`matchMedia("(hover: none), (pointer: coarse)")`)).toBe(false);
  });
});
