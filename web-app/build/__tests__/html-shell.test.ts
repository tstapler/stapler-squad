import { readFileSync } from "node:fs";
import path from "node:path";
import { injectFontPreload, injectThemeBoot } from "../html";
import { runInBun } from "./helpers/run-in-bun";

const WEB_APP = path.resolve(__dirname, "../..");
const MAP = { matrix: "m1", cyberpunk77: "c1", wh40k: "w1", clean: "cl", light: "l1", dark: "d1" };
const fontsCss = readFileSync(path.join(WEB_APP, "src/app/fonts.css"), "utf8");
const indexHtml = readFileSync(path.join(WEB_APP, "index.html"), "utf8");
const built = (runInBun("build.bun.ts", ["ok"]).json as { html: string; css: string }).html;

describe("index.html source", () => {
  it("carries the viewport, description and both placeholders, and no static <title> (React Router renders it)", () => {
    expect(indexHtml).toContain('<html lang="en">');
    expect(indexHtml).toContain('content="width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover"');
    expect(indexHtml).toContain('name="description" content="Manage your AI agent sessions"');
    expect(indexHtml).not.toMatch(/<title/i);
    expect(indexHtml).not.toContain('rel="manifest"');
    expect(indexHtml.match(/<!--THEME_BOOT-->/g)).toHaveLength(1);
    expect(indexHtml.match(/<!--FONT_PRELOAD-->/g)).toHaveLength(1);
  });
});

describe("fonts.css", () => {
  it("defines the four family variables with fallback stacks", () => {
    for (const v of ["--font-jetbrains-mono", "--font-rajdhani", "--font-cinzel", "--font-inter"]) {
      expect(fontsCss).toMatch(new RegExp(`${v}:[^;]*(monospace|sans-serif|serif)`));
    }
  });

  it("declares every vendored woff2 once, with font-display swap and URLs into /_next/static/media/", () => {
    const urls = [...fontsCss.matchAll(/url\("([^"]+)"\)/g)].map((m) => m[1]);
    expect(urls.every((u) => u.startsWith("/_next/static/media/") && u.endsWith(".woff2"))).toBe(true);
    expect(urls).toHaveLength(8);
    expect(fontsCss.match(/font-display: swap/g)).toHaveLength(8);
    expect(fontsCss).toMatch(/Cinzel[\s\S]*?font-weight: 400 700/);
    expect(fontsCss).toMatch(/JetBrains Mono[\s\S]*?font-style: italic/);
    expect(fontsCss.match(/font-family: "Rajdhani"/g)).toHaveLength(4);
  });
});

describe("emitted HTML (fixture build)", () => {
  it("keeps the head content, preloads the primary font with crossorigin, and has the theme script before the stylesheet", () => {
    expect(built).toContain('<html lang="en" class="_1b1axoi6">');
    expect(built).toContain("viewport-fit=cover");
    expect(built).toMatch(/<link rel="preload" as="font" type="font\/woff2" crossorigin href="\/_next\/static\/media\/inter-variable\.[0-9a-f]{8}\.woff2">/);
    expect(built).not.toMatch(/<title/i);
    expect(built).not.toContain("<!--");
  });

  it("makes the stylesheet a render-blocking <link> in <head>, not a module-injected style", () => {
    const head = /<head>([\s\S]*?)<\/head>/.exec(built)?.[1] ?? "";
    expect(head).toMatch(/<link rel="stylesheet"[^>]*href="\/_next\/static\/chunks\/index-[a-z0-9]+\.css"/);
    expect(head.indexOf("<script>(function(){")).toBeLessThan(head.indexOf('<link rel="stylesheet"'));
  });
});

describe("placeholder injection", () => {
  const shell = '<html lang="en"><head><!--FONT_PRELOAD--><!--THEME_BOOT--></head></html>';

  it("replaces each placeholder exactly once and leaves none", () => {
    const out = injectFontPreload(injectThemeBoot(shell, MAP), ["f.abc.woff2"]);
    expect(out).not.toContain("<!--");
    expect(out.match(/<script>/g)).toHaveLength(1);
    expect(out).toContain('href="/_next/static/media/f.abc.woff2"');
  });

  it("fails when the placeholder was stripped or duplicated, as a minifier might", () => {
    expect(() => injectThemeBoot("<html><head></head></html>", MAP)).toThrow(/found 0/);
    expect(() => injectThemeBoot(`${shell}<!--THEME_BOOT-->`, MAP)).toThrow(/found 2/);
  });
});
