// Gate S4 measurements for react-router 7.18.4. Run: bun router.spec.ts  (needs `bun build.ts` + `bun serve.ts dist 62875`).
// Not a Playwright-test file: a plain script that writes results.json; each section is one S4 acceptance criterion.
import { chromium, type Page } from "playwright-core";
import { writeFileSync } from "node:fs";
import { ROUTE_META } from "../../../../web-app/src/app/route-meta";

const BASE = process.env.S4_BASE ?? "http://127.0.0.1:62875";
const CHROME = process.env.S4_CHROME ?? `${process.env.HOME}/.cache/ms-playwright/chromium-1208/chrome-linux64/chrome`;
type Cfg = { scroll?: "window" | "main"; restoration?: boolean; flags?: Record<string, boolean>; flagDelayMs?: number };

const browser = await chromium.launch({ executablePath: CHROME, args: ["--no-sandbox"] });
const results: Record<string, unknown> = { router: "react-router@7.18.4", chromium: browser.version(), base: BASE };
const ALL_ON = { backlog: true, changelog: true };
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function open(cfg: Cfg, url: string, viewport = { width: 1280, height: 800 }) {
  const ctx = await browser.newContext({ viewport });
  const page = await ctx.newPage();
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  await page.addInitScript((c) => ((window as unknown as { __S4: unknown }).__S4 = c), cfg);
  await page.goto(BASE + url);
  await page.waitForSelector("h1", { timeout: 5000 }).catch(() => {});
  return { page, ctx, errors };
}
const histLen = (p: Page) => p.evaluate(() => history.length);
const loc = (p: Page) => p.evaluate(() => location.pathname + location.search + location.hash);
const settle = () => wait(250);

// ---- 1. Matching ----
{
  const { page, ctx, errors } = await open({ flags: ALL_ON }, "/");
  const paths = Object.keys(ROUTE_META);
  const cold: { path: string; plain: string | null; slash: string | null }[] = [];
  for (const p of paths) {
    const row = { path: p, plain: null as string | null, slash: null as string | null };
    for (const [k, url] of [["plain", p], ["slash", p === "/" ? "/" : `${p}/`]] as const) {
      await page.goto(BASE + url);
      await page.waitForSelector("[data-route]", { timeout: 5000 }).catch(() => {});
      row[k] = await page.evaluate(() => document.querySelector("[data-route]")?.getAttribute("data-route") ?? null);
    }
    cold.push(row);
  }
  const mismatches = cold.filter((r) => r.plain !== r.path || r.slash !== r.path);
  const settingsH1 = await (async () => {
    const out: Record<string, string | null> = {};
    for (const u of ["/settings", "/settings/"]) {
      await page.goto(BASE + u);
      await page.waitForSelector("h1");
      out[u] = await page.textContent("h1");
    }
    return out;
  })();
  // client navigation to a trailing-slash href
  await page.goto(BASE + "/");
  await page.waitForSelector("[data-route]");
  await page.click("#l-settings-slash");
  await page.waitForSelector("[data-route='/settings']");
  const clientSlash = await page.evaluate(() => ({ pathname: location.pathname, h1: document.querySelector("h1")?.textContent, rrLocation: (window as any).__router.state.location.pathname }));
  // unknown path with and without slash hits the catch-all
  await page.goto(BASE + "/nope/");
  await page.waitForSelector("h1");
  const notFound = await page.textContent("h1");
  results.matching = { routesTested: paths.length, coldLoadsPlainAndSlash: paths.length * 2, mismatches, settingsH1, clientSlash, notFound, pageErrors: errors };
  await ctx.close();
}

// ---- 2. Titles on client navigation ----
{
  const { page, ctx } = await open({ flags: ALL_ON }, "/");
  const t0 = await page.title();
  await page.click("#l-history");
  await page.waitForSelector("[data-route='/history']");
  await settle();
  const t1 = await page.title();
  const log = await page.evaluate(() => (window as any).__s4log);
  // walk every route by programmatic client navigation; compare document.title with ROUTE_META
  const paths = Object.keys(ROUTE_META);
  const bad: unknown[] = [];
  for (const p of paths) {
    await page.evaluate((to) => (window as any).__router.navigate(to), p);
    await page.waitForSelector(`[data-route='${p}']`, { timeout: 5000 }).catch(() => {});
    const got = await page.title();
    if (got !== ROUTE_META[p].title) bad.push({ p, got, want: ROUTE_META[p].title });
  }
  // title of the trailing-slash URL via client nav, and head <title> count
  await page.evaluate(() => (window as any).__router.navigate("/history/"));
  await settle();
  const slashTitle = await page.title();
  const titleTags = await page.evaluate(() => [...document.querySelectorAll("title")].map((t) => ({ parent: t.parentElement?.tagName, text: t.textContent })));
  results.titles = { initialTitle: t0, afterClientNavToHistory: t1, wanted: "History - Stapler Squad", announcerReadsTitleAfterCommit: log.announced, layoutEffectTitlesPerNav: log.effectTitles, routesWalked: paths.length, titleMismatches: bad, trailingSlashClientNavTitle: slashTitle, titleElements: titleTags };
  await ctx.close();
}

// ---- 3. Scroll container / ScrollRestoration ----
async function scrollMatrix(scroll: "window" | "main", restoration: boolean, keep = false) {
  const { page, ctx } = await open({ scroll, restoration }, "/");
  const pos = () => page.evaluate(() => ({ win: Math.round(window.scrollY), main: Math.round(document.getElementById("scroller")!.scrollTop) }));
  const setScroll = (y: number) => page.evaluate(([s, v]) => { if (s === "window") window.scrollTo(0, v as number); else document.getElementById("scroller")!.scrollTop = v as number; }, [scroll, y] as const);
  await setScroll(1200);
  await settle();
  const before = await pos();
  await page.evaluate((sel) => (document.querySelector(sel) as HTMLElement).click(), keep ? "#l-history-keep" : "#l-history"); // JS click: Playwright click would scroll the window to the link first
  await page.waitForSelector("[data-route='/history']");
  await settle();
  const afterForward = await pos();
  await setScroll(300);
  await settle();
  await page.goBack();
  await page.waitForSelector("[data-route='/']");
  await settle();
  const afterBack = await pos();
  await ctx.close();
  return { scroll, restoration, keep, before, afterForward, afterBack };
}
const scrollRuns = [] as unknown[];
for (const scroll of ["window", "main"] as const) for (const restoration of [false, true]) scrollRuns.push(await scrollMatrix(scroll, restoration));
scrollRuns.push(await scrollMatrix("window", true, true));
results.prototypeScroll = scrollRuns;

// ---- 4. Push vs replace matrix ----
async function lenDelta(label: string, setup: string, act: (p: Page) => Promise<void>) {
  const { page, ctx } = await open({ flags: { backlog: true } }, setup);
  const before = { len: await histLen(page), url: await loc(page) };
  await act(page);
  await settle();
  const after = { len: await histLen(page), url: await loc(page) };
  await ctx.close();
  return { label, before: before.url, after: after.url, historyLengthDelta: after.len - before.len, kind: after.len - before.len === 1 ? "push" : after.len === before.len ? "replace/no-new-entry" : "other" };
}
const nav = (to: string, opts?: unknown) => (p: Page) => p.evaluate(([t, o]) => (window as any).__router.navigate(t, o), [to, opts] as const).then(() => {});
const click = (sel: string) => (p: Page) => p.click(sel);
const matrix = [
  await lenDelta("router.push -> navigate(to)", "/unfinished?tab=a", nav("/history")),
  await lenDelta("router.replace -> navigate(to,{replace:true})", "/unfinished?tab=a", nav("/history", { replace: true })),
  await lenDelta("Link default (different URL)", "/", click("#l-history")),
  await lenDelta("Link replace (different URL)", "/", click("#l-history-replace")),
  await lenDelta("Link to the SAME url (default)", "/unfinished?tab=a", click("#l-unf")),
  await lenDelta("navigate(sameUrl) push", "/unfinished?tab=a", nav("/unfinished?tab=a")),
  await lenDelta("navigate('?tab=b') search-only push", "/unfinished?tab=a", nav("?tab=b")),
  await lenDelta("ReplaceSearch -> navigate('?tab=b',{replace:true})", "/unfinished?tab=a", nav("?tab=b", { replace: true })),
  await lenDelta("navigate('#x') hash-only", "/unfinished?tab=a", nav("#x")),
  await lenDelta("setSearchParams default", "/unfinished?tab=a", click("#set-default")),
  await lenDelta("setSearchParams {replace:true}", "/unfinished?tab=a", click("#set-replace")),
  await lenDelta("<Redirect> = <Navigate to> default (navigate to /__redir; 2 = redirect pushed, 1 = replaced it)", "/", nav("/__redir")),
  await lenDelta("<Redirect> = <Navigate to replace> (navigate to /__redir-replace)", "/", nav("/__redir-replace")),
];
// back(): navigate(-1) after a push
{
  const { page, ctx } = await open({}, "/");
  await page.click("#l-history");
  await page.waitForSelector("[data-route='/history']");
  const len = await histLen(page);
  await page.evaluate(() => (window as any).__router.navigate(-1));
  await page.waitForSelector("[data-route='/']");
  matrix.push({ label: "back() -> navigate(-1)", before: "/history", after: await loc(page), historyLengthDelta: (await histLen(page)) - len, kind: "traverse (length unchanged, index -1)" });
  await ctx.close();
}
results.pushReplaceMatrix = matrix;

// ---- 5. raw replaceState vs ReplaceSearch ----
{
  const { page, ctx } = await open({}, "/unfinished?tab=a");
  const out: Record<string, unknown> = {};
  out.initialTab = await page.textContent("#tab");
  const len0 = await histLen(page);
  await page.evaluate(() => history.replaceState(history.state, "", "?tab=b"));
  await wait(500);
  out.rawReplace = { locationSearch: await page.evaluate(() => location.search), useSearchParamsTab: await page.textContent("#tab"), rrRouterStateSearch: await page.evaluate(() => (window as any).__router.state.location.search), historyLengthDelta: (await histLen(page)) - len0 };
  // realistic follow-up: facade ReplaceSearch to the value the URL already shows
  await page.evaluate(() => (window as any).__router.navigate("?tab=b", { replace: true }));
  await wait(300);
  out.replaceSearchAfterRaw = { useSearchParamsTab: await page.textContent("#tab"), locationSearch: await page.evaluate(() => location.search), historyLengthDelta: (await histLen(page)) - len0 };
  await ctx.close();
  const f = await open({}, "/unfinished?tab=a");
  const len1 = await histLen(f.page);
  await f.page.evaluate(() => (window as any).__router.navigate("?tab=b", { replace: true }));
  await wait(300);
  out.replaceSearchClean = { useSearchParamsTab: await f.page.textContent("#tab"), locationSearch: await f.page.evaluate(() => location.search), historyLengthDelta: (await histLen(f.page)) - len1 };
  await f.ctx.close();
  results.searchParams = out;
}

// ---- 6. Foreign history.state (ssqTray) ----
{
  const { page, ctx } = await open({}, "/unfinished?tab=a");
  const st = () => page.evaluate(() => history.state);
  const out: Record<string, unknown> = { stateAfterLoad: await st() };
  // mark current entry the way pushHistoryMarkerInHistory(replace=true) does
  await page.evaluate(() => history.replaceState({ ...(history.state ?? {}), ssqTray: true }, ""));
  out.afterMarkReplace = await st();
  await page.evaluate(() => (window as any).__router.navigate("/history"));
  await settle();
  out.afterRRPush = { state: await st() };
  await page.goBack();
  await page.waitForSelector("[data-route='/unfinished']");
  await settle();
  out.afterBack = { state: await st(), markerRetained: (await st())?.ssqTray === true };
  // RR replace on a marked entry
  await page.evaluate(() => (window as any).__router.navigate("?tab=c", { replace: true }));
  await settle();
  out.afterRRReplaceOnMarkedEntry = { state: await st(), markerRetained: (await st())?.ssqTray === true };
  // facade-style fix: re-merge the marker after replace
  await page.evaluate(() => history.replaceState({ ...(history.state ?? {}), ssqTray: true }, ""));
  await page.evaluate(() => (window as any).__router.navigate("?tab=d", { replace: true }).then(() => history.replaceState({ ...(history.state ?? {}), ssqTray: true }, "")));
  await settle();
  out.afterRRReplaceThenRemerge = { state: await st() };
  // raw tray push (spread of RR state), then back and forward
  const pre = await page.evaluate(() => ({ len: history.length, idx: history.state?.idx }));
  await page.evaluate(() => history.pushState({ ...(history.state ?? {}), ssqTray: true }, ""));
  const pushed = await page.evaluate(() => ({ len: history.length, state: history.state, rrPath: (window as any).__router.state.location.pathname + (window as any).__router.state.location.search }));
  await page.goBack();
  await settle();
  const back = await page.evaluate(() => ({ state: history.state, url: location.search, rrSearch: (window as any).__router.state.location.search, tab: document.getElementById("tab")?.textContent }));
  await page.goForward();
  await settle();
  const fwd = await page.evaluate(() => ({ state: history.state, markerRetained: history.state?.ssqTray === true, rrSearch: (window as any).__router.state.location.search }));
  out.rawTrayPushBackForward = { pre, pushed, back, fwd };
  await ctx.close();
  results.foreignState = out;
}

// ---- 6b. Clean tray-entry sequence: pushTrayEntry (raw pushState with spread state), back, forward ----
{
  const { page, ctx } = await open({}, "/unfinished?tab=a");
  const snap = () => page.evaluate(() => ({ len: history.length, state: history.state, url: location.pathname + location.search, rr: (window as any).__router.state.location.pathname + (window as any).__router.state.location.search, rrAction: (window as any).__router.state.historyAction, renders: (window as any).__s4log.renders }));
  const s0 = await snap();
  await page.evaluate(() => history.pushState({ ...(history.state ?? {}), ssqTray: true }, ""));
  const s1 = await snap();
  await page.goBack();
  await settle();
  const s2 = await snap();
  await page.goForward();
  await settle();
  const s3 = await snap();
  // and an RR push on top of the tray entry, then back
  await page.evaluate(() => (window as any).__router.navigate("/history"));
  await settle();
  const s4 = await snap();
  await page.goBack();
  await settle();
  const s5 = await snap();
  results.trayEntryClean = { s0_load: s0, s1_afterRawTrayPush: s1, s2_afterBack: s2, s3_afterForward: s3, s4_afterRRPushOnTray: s4, s5_afterBackToTray: s5 };
  await ctx.close();
}

// ---- 7. FeatureGuard parity (compare with next-results.json guardRows, measured on the real Next app) ----
{
  const guards = Object.entries(ROUTE_META).filter(([, m]) => m.guard).map(([p, m]) => ({ path: p, flag: m.guard! }));
  const probe = (p: Page) => p.evaluate(() => ({ path: location.pathname, mainText: (document.getElementById("main-content")?.innerText ?? "").trim().length, h1: document.querySelector("h1")?.textContent ?? null, len: history.length }));
  const rows: unknown[] = [];
  for (const g of guards) {
    for (const enabled of [true, false]) {
      for (const via of ["client-nav", "cold-load"] as const) {
        const cfg: Cfg = { flags: { [g.flag]: enabled }, flagDelayMs: 800 };
        let page: Page; let ctx: Awaited<ReturnType<typeof open>>["ctx"]; let len0: number | null; let early: unknown;
        if (via === "client-nav") {
          ({ page, ctx } = await open(cfg, "/history"));
          await wait(1200); // flag store finished loading
          len0 = (await probe(page)).len;
          await page.evaluate((to) => (window as any).__router.navigate(to), g.path);
          await wait(250);
          early = await probe(page);
        } else {
          ({ page, ctx } = await open(cfg, g.path)); // open() waits for h1 up to 5 s; flag store still loading at +150 ms is checked next
          len0 = null;
          early = null;
          await ctx.close();
          const c2 = await browser.newContext();
          const p2 = await c2.newPage();
          await p2.addInitScript((c) => ((window as any).__S4 = c), cfg);
          await p2.goto(BASE + g.path, { waitUntil: "commit" });
          await wait(450);
          early = await probe(p2).catch((e) => String(e));
          await wait(1800);
          const late = await probe(p2);
          rows.push({ guardPath: g.path, flag: g.flag, enabled, via, early, late, historyLengthDeltaFromBefore: null, replaceCalls: await p2.evaluate(() => (window as any).__s4log.replaceCalls) });
          await c2.close();
          continue;
        }
        await wait(600);
        const late = await probe(page);
        rows.push({ guardPath: g.path, flag: g.flag, enabled, via, early, late, historyLengthDeltaFromBefore: late.len - (len0 as number), replaceCalls: await page.evaluate(() => (window as any).__s4log.replaceCalls) });
        await ctx.close();
      }
    }
  }
  results.guardParity = { guards, rows };
}

await browser.close();
writeFileSync(`${import.meta.dir}/results.json`, JSON.stringify(results, null, 2));
console.log(JSON.stringify(results, null, 1));
