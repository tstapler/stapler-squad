// Resolves Playwright from web-app/node_modules (run scripts with cwd = web-app/) and launches the cached
// Chromium 1208 build (playwright 1.57 wants 1200, which is not installed on this host).
import { createRequire } from "node:module";
import { homedir } from "node:os";
import { join, resolve } from "node:path";

const { chromium } = createRequire(resolve("package.json"))("@playwright/test");
const executablePath = join(homedir(), ".cache/ms-playwright/chromium_headless_shell-1208/chrome-headless-shell-linux64/chrome-headless-shell");

export const launch = () => chromium.launch({ executablePath });
