import type { ThemeMap } from "../../src/styles/theme-boot-script";
import type { VeCompiler } from "../plugins/vanilla-extract";
import type { StageTiming } from "./runner";

export interface BuildOptions {
  /** web-app directory (or a fixture app laid out the same way). */
  root: string;
  /** Final output; defaults to <root>/out. The minified size build writes elsewhere so it never reaches the embed. */
  outDir?: string;
  minify: boolean;
  /** WEB_MINIFY was set by the caller; an unminified build without it fails verify-output. */
  minifyExplicit: boolean;
  env: Record<string, string | undefined>;
  /** Defaults to <root>/.build/stats.json, outside out/ so it never ships. */
  statsPath?: string;
  /** Defaults to the repo's public allowlist; fixtures pass their own. */
  allowlistPath?: string;
  budgetsPath?: string;
  /** Skip the byte budgets (structural checks still run). */
  enforceBytes: boolean;
  /** Paths relative to root; overridable so a fixture app can stay tiny. */
  entryHtml?: string;
  themeFile?: string;
}

export interface BuildContext {
  opts: BuildOptions;
  root: string;
  srcDir: string;
  entryHtml: string;
  themeFile: string;
  publicDir: string;
  fontsDir: string;
  outDir: string;
  tmpDir: string;
  statsPath: string;
  ve: VeCompiler;
  timings: StageTiming[];
  themeMap?: ThemeMap;
  /** Unhashed font file name to its content-hashed copy under /_next/static/media/, set by the bundle stage. */
  fontFiles: Map<string, string>;
  /** Absolute paths of the files Bun wrote, set by the bundle stage. */
  emitted: { path: string; kind: string }[];
  /** JS files reachable from the /login route entry, when the metafile allows computing them. */
  loginClosure?: string[];
  warnings: string[];
  startedAt: number;
}
