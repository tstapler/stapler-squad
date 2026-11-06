// Gate S3, Task 0.4c/0.4e: computed-style ParityDiff between a Next build and a Bun build, plus the in-session
// navigation walk and the CSS rule-order census. Run from web-app/ with both builds served (see serve.ts):
//   node ../project_plans/web-app-bun-build/spikes/s3/parity-diff.mjs matrix <nextBase> <bunBase> <themeSourceHtml> <out.json>
//   node .../parity-diff.mjs cold   <nextBase> <bunBase> <themeSourceHtml> <out.json> [snapshots.json]
//   node .../parity-diff.mjs walk   <nextBase> <bunBase> <themeSourceHtml> <out.json>      (Bun in-session walk vs Bun cold vs Next cold)
//   node .../parity-diff.mjs census <nextCss> <bunCss> <themeSourceHtml> <out.json>
// Noise control: every Next snapshot is taken twice (two contexts); a (config, element, property) that already differs
// between those two Next loads is dropped from the Bun-vs-Next diff and counted as noise.
import { readFileSync, writeFileSync } from "node:fs";
import { launch } from "./pw.mjs";
import { extractRules } from "./css-rules.mjs";

const [mode, argA, argB, themeHtml, outFile] = process.argv.slice(2);
const themeMap = JSON.parse(readFileSync(themeHtml, "utf8").match(/var m=(\{[^}]*\})/)[1]);
const THEMES = Object.keys(themeMap);
const MATRIX_ROUTES = ["/", "/settings", "/backlog", "/review-queue", "/history"];
const VIEWPORTS = [
  { width: 390, height: 844 },
  { width: 1280, height: 800 },
];
const ALL_ROUTES = JSON.parse(readFileSync(new URL("./routes.json", import.meta.url), "utf8"));
const FOCUS_COUNT = 5;
const CONCURRENCY = 8;
const MAX_LISTED = 25;

// ---- in-page helpers (serialised into the page)
const pageHelpers = () => {
  const props = (el) => {
    const cs = getComputedStyle(el);
    const out = {};
    for (let i = 0; i < cs.length; i++) if (!cs[i].startsWith("--")) out[cs[i]] = cs.getPropertyValue(cs[i]); // custom properties are compared once, resolved, in vars()
    return out;
  };
  window.__ssq = {
    props,
    snap() {
      const els = {};
      const seen = {};
      for (const el of [...document.querySelectorAll("[data-testid]")].slice(0, 400)) {
        const id = el.getAttribute("data-testid").replace(/[0-9a-f]{8}(?:-[0-9a-f]{4}){0,4}/g, "ID");
        seen[id] = (seen[id] ?? 0) + 1;
        els[`${id}#${seen[id]}`] = props(el);
      }
      return els;
    },
    vars() {
      const names = new Set();
      const walk = (rules) => {
        for (const r of rules) {
          if (r.style) for (const p of r.style) if (p.startsWith("--")) names.add(p);
          if (r.cssRules) walk(r.cssRules);
        }
      };
      for (const s of document.styleSheets) {
        try {
          walk(s.cssRules);
        } catch {
          /* cross-origin sheet */
        }
      }
      const root = getComputedStyle(document.documentElement);
      const probe = document.body.appendChild(document.createElement("div"));
      const canon = (v) => {
        for (const prop of ["color", "boxShadow", "transition", "width"]) {
          probe.style[prop] = "";
          probe.style[prop] = v;
          if (probe.style[prop]) return getComputedStyle(probe)[prop];
        }
        return v.replace(/\s+/g, "").toLowerCase().replace(/(^|[^\d])0\./g, "$1.");
      };
      const out = {};
      for (const n of [...names].sort()) out[n] = canon(root.getPropertyValue(n).trim());
      probe.remove();
      return out;
    },
  };
};

async function newPage(browser, base, viewport, route) {
  const ctx = await browser.newContext({ viewport });
  const page = await ctx.newPage();
  const pending = { n: 0 };
  page.on("request", () => pending.n++);
  page.on("requestfinished", () => pending.n--);
  page.on("requestfailed", () => pending.n--);
  page.__pending = pending;
  await page.addInitScript(() => {
    localStorage.setItem("stapler-theme", "clean");
    // Suppress the first-run onboarding tour (useOnboarding.ts fires it ~800ms after mount on any
    // session lacking this key): a long enough walk/settle otherwise crosses that timer, opening a
    // real Radix Dialog whose DismissableLayer legitimately locks `body` with `pointer-events: none`
    // until dismissed (confirmed restored correctly on Escape) — same precondition as
    // tests/e2e/terminal-stress/helpers.ts's setupStressTestPage. See spike-results.md's "### 10".
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(base + (route === "/" ? "/" : route + "/"), { waitUntil: "load" });
  await page.waitForSelector("#main-content", { timeout: 10000 }).catch(() => {});
  await page.evaluate(pageHelpers);
  await settle(page, 500);
  return page;
}

async function settle(page, minMs = 250) {
  let quietSince = Date.now();
  const deadline = Date.now() + 6000;
  while (Date.now() < deadline) {
    if (page.__pending.n > 0) quietSince = Date.now();
    else if (Date.now() - quietSince >= minMs) return;
    await page.waitForTimeout(50);
  }
}

const setTheme = (page, name) =>
  page.evaluate(
    ([map, cls]) => {
      const html = document.documentElement;
      for (const c of Object.values(map)) html.classList.remove(c);
      html.classList.add(cls);
    },
    [themeMap, themeMap[name]],
  );

const snapshot = (page) => page.evaluate(() => ({ els: window.__ssq.snap(), vars: window.__ssq.vars(), path: location.pathname }));

async function interactionSnaps(page) {
  const out = {};
  await page.mouse.move(0, 0);
  await page.evaluate(() => document.activeElement?.blur?.());
  const handles = [];
  for (let k = 0; k < FOCUS_COUNT; k++) {
    await page.keyboard.press("Tab");
    const h = await page.evaluateHandle(() => (document.activeElement && document.activeElement !== document.body ? document.activeElement : null));
    const el = h.asElement();
    if (!el) break;
    handles.push(el);
    out[`focus#${k}`] = await el.evaluate((e) => ({ id: `${e.tagName}|${(e.getAttribute("aria-label") ?? e.textContent ?? "").trim().slice(0, 30)}`, props: window.__ssq.props(e), matches: e.matches(":focus-visible") }));
  }
  await page.evaluate(() => document.activeElement?.blur?.());
  for (let k = 0; k < handles.length; k++) {
    try {
      await handles[k].hover({ force: true, timeout: 1500 });
      out[`hover#${k}`] = await handles[k].evaluate((e) => ({ id: `${e.tagName}|${(e.getAttribute("aria-label") ?? e.textContent ?? "").trim().slice(0, 30)}`, props: window.__ssq.props(e), matches: e.matches(":hover") }));
    } catch (e) {
      out[`hover#${k}`] = { skipped: String(e).slice(0, 80) };
    }
  }
  await page.mouse.move(0, 0);
  return out;
}

// ---- diffing
function diffProps(a, b, ctx, sink, onlyKeys) {
  for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
    if (!(key in a) || !(key in b)) {
      sink.structure.push({ ctx, key, side: key in a ? "onlyNext" : "onlyBun" });
      continue;
    }
    for (const p of new Set([...Object.keys(a[key]), ...Object.keys(b[key])])) {
      if (a[key][p] !== b[key][p]) sink.props.push({ ctx, key, prop: p, a: a[key][p], b: b[key][p] });
    }
  }
  void onlyKeys;
}
const diffVars = (a, b, ctx, sink) => {
  for (const n of new Set([...Object.keys(a), ...Object.keys(b)])) if (a[n] !== b[n]) sink.vars.push({ ctx, name: n, a: a[n], b: b[n] });
};
const flatten = (snap, ctx) => {
  const els = { ...snap.els };
  return els;
};
const newSink = () => ({ props: [], structure: [], vars: [], identity: [], skipped: 0 });

function compareStates(ctx, nextA, nextB, bun, sink, noise) {
  // element snapshots
  const control = newSink();
  diffProps(flatten(nextA), flatten(nextB), ctx, control);
  diffVars(nextA.vars, nextB.vars, ctx, control);
  const noisy = new Set(control.props.map((d) => `${d.key}|${d.prop}`));
  const noisyStruct = new Set(control.structure.map((d) => d.key));
  const noisyVars = new Set(control.vars.map((d) => d.name));
  noise.props += control.props.length;
  noise.structure += control.structure.length;
  noise.vars += control.vars.length;
  const raw = newSink();
  diffProps(flatten(nextA), flatten(bun), ctx, raw);
  diffVars(nextA.vars, bun.vars, ctx, raw);
  sink.props.push(...raw.props.filter((d) => !noisy.has(`${d.key}|${d.prop}`)));
  sink.structure.push(...raw.structure.filter((d) => !noisyStruct.has(d.key)));
  sink.vars.push(...raw.vars.filter((d) => !noisyVars.has(d.name)));
}

function compareInteraction(ctx, nA, nB, bun, sink, noise) {
  for (const k of new Set([...Object.keys(nA), ...Object.keys(bun)])) {
    const a = nA[k];
    const c = nB[k];
    const b = bun[k];
    if (!a || !b || a.skipped || b.skipped || !c || c.skipped) {
      sink.skipped++;
      continue;
    }
    if (a.id !== b.id) {
      sink.identity.push({ ctx, k, next: a.id, bun: b.id });
      continue;
    }
    const noisy = new Set();
    for (const p of Object.keys(a.props)) if (a.props[p] !== c.props[p]) noisy.add(p);
    noise.props += noisy.size;
    for (const p of Object.keys(a.props)) if (!noisy.has(p) && a.props[p] !== b.props[p]) sink.props.push({ ctx: `${ctx} ${k}`, key: a.id, prop: p, a: a.props[p], b: b.props[p] });
    if (a.matches !== b.matches) sink.identity.push({ ctx, k, note: "state pseudo-class match differs", next: a.matches, bun: b.matches });
  }
}

// Allow-list: each entry needs a reason that is a URL or a quoted CSS rule (a bare string is rejected at startup).
const ALLOW = [
  {
    prop: "background-position",
    a: "0% 0%",
    b: "0px 0px",
    reason: 'Bun minifies `background:none` to `background:0 0` (e.g. `.ub2m1b6{...background:0 0;...}`): same paint, computed position `0px 0px` instead of the initial `0% 0%`',
  },
];
for (const e of ALLOW) if (!/^https?:\/\//.test(e.reason) && !/[{}]/.test(e.reason)) throw new Error(`allow-list entry for ${e.prop} lacks a linked reason`);

function summarize(full) {
  const explained = {};
  const sink = { ...full, props: full.props.filter((d) => {
    const hit = ALLOW.find((e) => e.prop === d.prop && e.a === d.a && e.b === d.b);
    if (hit) explained[hit.prop] = (explained[hit.prop] ?? 0) + 1;
    return !hit;
  }) };
  const byProp = {};
  for (const d of sink.props) byProp[d.prop] = (byProp[d.prop] ?? 0) + 1;
  const byCtx = {};
  for (const d of [...sink.props, ...sink.structure, ...sink.vars]) byCtx[d.ctx] = (byCtx[d.ctx] ?? 0) + 1;
  return {
    explainedByAllowList: explained,
    propDiffs: sink.props.length,
    structureDiffs: sink.structure.length,
    varDiffs: sink.vars.length,
    identityDiffs: sink.identity.length,
    skippedInteractions: sink.skipped,
    topProps: Object.entries(byProp).sort((x, y) => y[1] - x[1]).slice(0, 12),
    contextsWithDiffs: Object.keys(byCtx).length,
    sampleProps: sink.props.slice(0, MAX_LISTED * 3),
    sampleStructure: sink.structure.filter((d) => !d.key.startsWith("route-announcer")).slice(0, MAX_LISTED * 3),
    routeAnnouncerOnlyBun: sink.structure.filter((d) => d.key.startsWith("route-announcer")).length,
    sampleVars: sink.vars.slice(0, MAX_LISTED),
    sampleIdentity: sink.identity.slice(0, MAX_LISTED),
    contextCounts: byCtx,
  };
}

async function pool(tasks, n) {
  const results = [];
  const queue = [...tasks];
  await Promise.all(
    Array.from({ length: n }, async () => {
      for (let t = queue.shift(); t; t = queue.shift()) results.push(await t());
    }),
  );
  return results;
}

// ---- modes
async function runMatrix(browser, nextBase, bunBase) {
  const sink = newSink();
  const noise = { props: 0, structure: 0, vars: 0 };
  let configs = 0;
  const tasks = [];
  for (const route of MATRIX_ROUTES) {
    for (const viewport of VIEWPORTS) {
      tasks.push(async () => {
        const [nA, nB, bn] = await Promise.all([newPage(browser, nextBase, viewport, route), newPage(browser, nextBase, viewport, route), newPage(browser, bunBase, viewport, route)]);
        const landing = { next: new URL(nA.url()).pathname, bun: new URL(bn.url()).pathname };
        for (const theme of THEMES) {
          const ctx = `${route} ${viewport.width}x${viewport.height} ${theme}`;
          await Promise.all([setTheme(nA, theme), setTheme(nB, theme), setTheme(bn, theme)]);
          await Promise.all([settle(nA, 100), settle(nB, 100), settle(bn, 100)]);
          const [sa, sb, sc] = [await snapshot(nA), await snapshot(nB), await snapshot(bn)];
          compareStates(ctx, sa, sb, sc, sink, noise);
          const [ia, ib, ic] = [await interactionSnaps(nA), await interactionSnaps(nB), await interactionSnaps(bn)];
          compareInteraction(ctx, ia, ib, ic, sink, noise);
          configs++;
        }
        await Promise.all([nA.context().close(), nB.context().close(), bn.context().close()]);
        return landing;
      });
    }
  }
  const landings = await pool(tasks, 4);
  return { configs, noiseDropped: noise, landings, ...summarize(sink) };
}

async function runCold(browser, nextBase, bunBase) {
  const sink = newSink();
  const noise = { props: 0, structure: 0, vars: 0 };
  const viewport = VIEWPORTS[1];
  const snaps = {};
  const landings = {};
  const classLists = new Set();
  const tasks = ALL_ROUTES.map((route) => async () => {
    const [nA, nB, bn] = await Promise.all([newPage(browser, nextBase, viewport, route), newPage(browser, nextBase, viewport, route), newPage(browser, bunBase, viewport, route)]);
    landings[route] = { next: new URL(nA.url()).pathname, bun: new URL(bn.url()).pathname };
    for (const theme of ["clean", "dark"]) {
      await Promise.all([setTheme(nA, theme), setTheme(nB, theme), setTheme(bn, theme)]);
      await Promise.all([settle(nA, 100), settle(nB, 100), settle(bn, 100)]);
      const [sa, sb, sc] = [await snapshot(nA), await snapshot(nB), await snapshot(bn)];
      compareStates(`${route} ${theme}`, sa, sb, sc, sink, noise);
      snaps[`${route} ${theme}`] = { next: sa, bun: sc };
      for (const l of await bn.evaluate(() => [...document.querySelectorAll("[class]")].map((e) => e.getAttribute("class")))) classLists.add(l);
    }
    await Promise.all([nA.context().close(), nB.context().close(), bn.context().close()]);
  });
  await pool(tasks, 4);
  writeFileSync(outFile.replace(/\.json$/, ".classlists.json"), JSON.stringify([...classLists]));
  const mismatchedLanding = Object.entries(landings).filter(([, v]) => v.next !== v.bun);
  return { routes: ALL_ROUTES.length, noiseDropped: noise, mismatchedLanding, snaps, ...summarize(sink) };
}

const mulberry32 = (seed) => () => {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};
const shuffled = (arr, seed) => {
  const rnd = mulberry32(seed);
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(rnd() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
};

async function runWalk(browser, nextBase, bunBase) {
  const viewport = VIEWPORTS[1];
  const meta = [...readFileSync(new URL("../../../../web-app/src/app/route-meta.ts", import.meta.url), "utf8").matchAll(/^\s+(?:path:\s*)?"(\/[^"]*)"/gm)].map((m) => m[1]);
  const SEEDS = [20261021, 1337, 424242];
  const orderings = [
    { name: "ascending", order: [...ALL_ROUTES].sort() },
    { name: "descending", order: [...ALL_ROUTES].sort().reverse() },
    { name: "routemeta-declaration", order: meta },
    ...SEEDS.map((s) => ({ name: `random-seed-${s}`, order: shuffled(ALL_ROUTES, s) })),
  ];
  // cold references: Next once, Bun twice (second Bun load is the Bun-vs-Bun noise control)
  const cold = {};
  await pool(
    ALL_ROUTES.map((route) => async () => {
      const [n, b1, b2] = await Promise.all([newPage(browser, nextBase, viewport, route), newPage(browser, bunBase, viewport, route), newPage(browser, bunBase, viewport, route)]);
      cold[route] = { next: await snapshot(n), bun: await snapshot(b1), bunB: await snapshot(b2) };
      await Promise.all([n.context().close(), b1.context().close(), b2.context().close()]);
    }),
    CONCURRENCY,
  );
  const results = [];
  for (const { name, order } of orderings) {
    const page = await newPage(browser, bunBase, viewport, "/");
    await page.evaluate(() => (window.__noReloadMarker = Math.random()));
    const marker = await page.evaluate(() => window.__noReloadMarker);
    const vsBunCold = newSink();
    const vsNext = newSink();
    const noiseB = { props: 0, structure: 0, vars: 0 };
    let reloads = 0;
    let finalCtx = null;
    for (const route of order) {
      await page.evaluate((p) => {
        history.pushState(null, "", p === "/" ? "/" : p + "/"); // "/" + "/" is a protocol-relative URL, not a path
        window.dispatchEvent(new PopStateEvent("popstate", { state: null }));
      }, route);
      await page.waitForTimeout(120);
      await settle(page, 200);
      if ((await page.evaluate(() => window.__noReloadMarker)) !== marker) {
        reloads++;
        await page.evaluate(pageHelpers);
        continue;
      }
      const snap = await snapshot(page);
      // Bun-vs-Bun: walk snapshot against cold Bun, dropping what already differs between two cold Bun loads
      const ref = cold[route];
      const c = newSink();
      diffProps(flatten(ref.bun), flatten(ref.bunB), route, c);
      const noisy = new Set(c.props.map((d) => `${d.key}|${d.prop}`));
      noiseB.props += c.props.length;
      const raw = newSink();
      diffProps(flatten(ref.bun), flatten(snap), `${name} ${route}`, raw);
      diffVars(ref.bun.vars, snap.vars, `${name} ${route}`, raw);
      vsBunCold.props.push(...raw.props.filter((d) => !noisy.has(`${d.key}|${d.prop}`)));
      vsBunCold.structure.push(...raw.structure);
      vsBunCold.vars.push(...raw.vars);
      const rawN = newSink();
      diffProps(flatten(ref.next), flatten(snap), `${name} ${route}`, rawN);
      vsNext.props.push(...rawN.props);
      vsNext.structure.push(...rawN.structure);
      if (route === order[order.length - 1]) finalCtx = { route, vsBunColdDiffs: raw.props.filter((d) => !noisy.has(`${d.key}|${d.prop}`)).length + raw.structure.length + raw.vars.length };
    }
    await page.context().close();
    results.push({ ordering: name, visited: order.length, reloadsDetected: reloads, final: finalCtx, vsBunCold: summarize(vsBunCold), vsNextColdUncontrolled: { propDiffs: vsNext.props.length, structureDiffs: vsNext.structure.length }, boundBunNoiseProps: noiseB.props });
  }
  return { seeds: SEEDS, orderings: results };
}

const decls = (body) => {
  const m = new Map();
  for (const part of body.split(/;(?![^(]*\))/)) {
    const k = part.indexOf(":");
    if (k > 0) m.set(part.slice(0, k).trim(), part.slice(k + 1).trim());
  }
  return m;
};
const specificity = (sel) => {
  const s = sel.replace(/\[[^\]]*\]/g, ".a").replace(/:(where|is|not|has)\(/g, (m) => (m === ":where(" ? ":w(" : "("));
  const ids = (s.match(/#[\w-]+/g) ?? []).length;
  const cls = (s.match(/\.[\w-]+|:(?!:)[\w-]+(?!\()/g) ?? []).length;
  const els = (s.match(/(^|[\s>+~(])[a-z][\w-]*/gi) ?? []).length;
  return `${ids},${cls},${els}`;
};
const classTokens = (sel) => new Set(sel.match(/\.[\w-]+/g) ?? []);

async function runCensus(nextCssFile, bunCssFile, browser, classListsFile) {
  const page = await browser.newPage();
  const next = await extractRules(page, readFileSync(nextCssFile, "utf8"));
  const bun = await extractRules(page, readFileSync(bunCssFile, "utf8"));
  const tag = (rules) => {
    const count = {};
    return rules.map((r) => {
      const key = `${r.ctx}|${r.sel}`;
      count[key] = (count[key] ?? 0) + 1;
      return { ...r, key, id: `${key}#${count[key]}`, nth: count[key] };
    });
  };
  const N = tag(next);
  const B = tag(bun);
  const bIds = new Map(B.map((r, i) => [r.id, i]));
  const bKeys = new Set(B.map((r) => r.key));
  const lis = (seq) => {
    const tails = [];
    for (const v of seq) {
      let lo = 0;
      let hi = tails.length;
      while (lo < hi) {
        const mid = (lo + hi) >> 1;
        if (tails[mid] < v) lo = mid + 1;
        else hi = mid;
      }
      tails[lo] = v;
    }
    return tails.length;
  };
  // strict: every (ctx|selector#occurrence) item
  const commonSeq = N.map((r) => bIds.get(r.id)).filter((x) => x !== undefined);
  // Next duplicates (2nd+ occurrence of a selector) that Bun does not have: exact copies of an earlier body are harmless merges
  const firstBody = new Map();
  const dup = { total: 0, exactCopyOfEarlier: 0, differentBody: [] };
  for (const r of N) {
    if (r.nth === 1) firstBody.set(r.key, [r.body]);
    else {
      firstBody.get(r.key).push(r.body);
      if (!bIds.has(r.id)) {
        dup.total++;
        if (firstBody.get(r.key).slice(0, -1).includes(r.body)) dup.exactCopyOfEarlier++;
        else if (dup.differentBody.length < 15) dup.differentBody.push({ key: r.key, body: r.body.slice(0, 120) });
      }
    }
  }
  // unique selector keys, first occurrence only
  const firstN = N.filter((r) => r.nth === 1).map((r) => bIds.get(`${r.key}#1`)).filter((x) => x !== undefined);
  // conflict candidates: same property, different values, same specificity, share a class token, same at-rule ctx, order flipped
  const pos = new Map(N.map((r, i) => [r.id, i]));
  const byProp = new Map();
  for (const r of N) {
    const bi = bIds.get(r.id);
    if (bi === undefined) continue;
    const spec = specificity(r.sel);
    const toks = classTokens(r.sel);
    for (const [prop, val] of decls(r.body)) {
      if (!byProp.has(prop)) byProp.set(prop, []);
      byProp.get(prop).push({ n: pos.get(r.id), b: bi, val, spec, toks, ctx: r.ctx, sel: r.sel });
    }
  }
  let candidates = 0;
  const examples = [];
  const propsAffected = {};
  for (const [prop, list] of byProp) {
    list.sort((x, y) => x.n - y.n);
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const x = list[i];
        const y = list[j];
        if (x.b < y.b || x.val === y.val || x.spec !== y.spec || x.ctx !== y.ctx) continue;
        let shared = false;
        for (const t of x.toks) if (y.toks.has(t)) shared = true;
        if (!shared) continue;
        candidates++;
        propsAffected[prop] = (propsAffected[prop] ?? 0) + 1;
        if (examples.length < 20) examples.push({ prop, nextOrder: [x.sel, y.sel], vals: [x.val, y.val] });
      }
    }
  }
  // merge check: a selector that Next emits several times and Bun once must fold to the same declarations
  const fold = (rules, key) => {
    const m = new Map();
    for (const r of rules) if (r.key === key) for (const [k, v] of decls(r.body)) m.set(k, v);
    return m;
  };
  const dupKeys = [...new Set(N.filter((r) => r.nth > 1 && !bIds.has(r.id)).map((r) => r.key))];
  const mergeDifferent = [];
  let mergeEqual = 0;
  for (const key of dupKeys) {
    const a = fold(N, key);
    const b = fold(B, key);
    const delta = [...new Set([...a.keys(), ...b.keys()])].filter((k) => a.get(k) !== b.get(k)).map((k) => `${k}: ${a.get(k)} | ${b.get(k)}`);
    if (delta.length === 0) mergeEqual++;
    else if (mergeDifferent.length < 15) mergeDifferent.push({ key, delta: delta.slice(0, 4) });
  }
  // co-applied class pairs (from real DOM class lists): single-class rules with the same property and different values
  // are the cascade-order-sensitive ones; count pairs whose relative order differs between the builds
  let coApplied = null;
  if (classListsFile) {
    const lists = JSON.parse(readFileSync(classListsFile, "utf8"));
    const single = (rules) => {
      const m = new Map();
      for (const r of rules) if (/^\.[\w-]+$/.test(r.sel) && r.ctx === "") (m.get(r.sel.slice(1)) ?? m.set(r.sel.slice(1), []).get(r.sel.slice(1))).push(r);
      return m;
    };
    const sn = single(N);
    const sb = single(B);
    const lastIdx = (rules, cls, rs) => rs.indexOf(rules.get(cls).at(-1));
    const pairs = new Set();
    for (const l of lists) {
      const cs = [...new Set(l.split(/\s+/))].filter((c) => sn.has(c) && sb.has(c));
      for (let i = 0; i < cs.length; i++) for (let j = i + 1; j < cs.length; j++) pairs.add(cs[i] < cs[j] ? `${cs[i]} ${cs[j]}` : `${cs[j]} ${cs[i]}`);
    }
    const flips = [];
    let overlapping = 0;
    for (const pr of pairs) {
      const [a, b] = pr.split(" ");
      const da = fold(sn.get(a), sn.get(a)[0].key);
      const db = fold(sn.get(b), sn.get(b)[0].key);
      const shared = [...da.keys()].filter((k) => db.has(k) && da.get(k) !== db.get(k));
      if (shared.length === 0) continue;
      overlapping++;
      const nOrder = lastIdx(sn, a, N) < lastIdx(sn, b, N);
      const bOrder = lastIdx(sb, a, B) < lastIdx(sb, b, B);
      if (nOrder !== bOrder) flips.push({ a, b, props: shared.slice(0, 4) });
    }
    coApplied = { distinctClassLists: lists.length, coAppliedPairs: pairs.size, pairsWithConflictingProperty: overlapping, orderFlipped: flips.length, flipExamples: flips.slice(0, 15) };
  }
  const byThemeVarRule = Object.entries(themeMap).map(([theme, cls]) => ({ theme, nextRules: N.filter((r) => r.sel.includes(`.${cls}`)).length, bunRules: B.filter((r) => r.sel.includes(`.${cls}`)).length }));
  return {
    nextRulesPerSelector: N.length,
    bunRulesPerSelector: B.length,
    strictItems: { common: commonSeq.length, longestInOrderRun: lis(commonSeq), outOfOrder: commonSeq.length - lis(commonSeq), onlyNext: N.filter((r) => !bIds.has(r.id)).length, onlyBun: B.length - commonSeq.length },
    uniqueSelectorKeys: { next: new Set(N.map((r) => r.key)).size, bun: new Set(B.map((r) => r.key)).size, nextKeysMissingInBun: [...new Set(N.map((r) => r.key))].filter((k) => !bKeys.has(k)).length, firstOccurrenceOutOfOrder: firstN.length - lis(firstN), firstOccurrenceCommon: firstN.length },
    nextDuplicatesMissingInBun: dup,
    duplicateMergeFold: { selectorsMergedByBun: dupKeys.length, foldedDeclarationsEqual: mergeEqual, foldedDeclarationsDiffer: dupKeys.length - mergeEqual, differExamples: mergeDifferent },
    coAppliedClassPairs: coApplied,
    orderFlippedConflictCandidates: { count: candidates, propsAffected, examples },
    perThemeClassRules: byThemeVarRule,
  };
}

const browser = await launch();
let result;
if (mode === "matrix") result = await runMatrix(browser, argA, argB);
else if (mode === "cold") result = await runCold(browser, argA, argB);
else if (mode === "walk") result = await runWalk(browser, argA, argB);
else if (mode === "census") result = await runCensus(argA, argB, browser, process.argv[7]);
else throw new Error(`unknown mode ${mode}`);
await browser.close();
const { snaps, ...rest } = result;
writeFileSync(outFile, JSON.stringify(rest, null, 1));
if (snaps) writeFileSync(outFile.replace(/\.json$/, ".snaps.json"), JSON.stringify(snaps));
console.log(JSON.stringify({ mode, ...Object.fromEntries(Object.entries(rest).filter(([, v]) => typeof v !== "object" || Array.isArray(v) === false && Object.keys(v).length < 6)) }, null, 1).slice(0, 2500));
