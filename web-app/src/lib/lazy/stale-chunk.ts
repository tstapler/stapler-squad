// Recovery for a lazy chunk that 404s after a deploy. Used by impl.bun.tsx only.
// Reloads once per build, unless the user has unsaved input, in which case a banner asks them to.

const FLAG_KEY = "ssq:stale-chunk-reload";
const BANNER_ID = "stale-chunk-banner";
const SETTLE_MS = 10_000;
const EXCLUDED = 'input[type="search"], [role="searchbox"], [data-draft-ignore]';
const TEXT_FIELDS =
  'input:not([type]), input[type="text"], input[type="email"], input[type="url"], input[type="tel"], input[type="password"], input[type="number"], textarea, [contenteditable]:not([contenteditable="false"])';

const CHUNK_ERROR = /Loading (CSS )?chunk|dynamically imported module|Importing a module script failed|ChunkLoadError/i;

export function isChunkLoadError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const { name, message } = err as { name?: unknown; message?: unknown };
  return name === "ChunkLoadError" || (typeof message === "string" && CHUNK_ERROR.test(message));
}

// Fallback when sessionStorage throws (private mode, blocked site data).
let memoryFlag: string | null = null;
let settleTimer: ReturnType<typeof setTimeout> | undefined;

function readFlag(): string | null {
  try {
    return sessionStorage.getItem(FLAG_KEY) ?? memoryFlag;
  } catch {
    return memoryFlag;
  }
}

function writeFlag(value: string | null): void {
  memoryFlag = value;
  try {
    if (value === null) sessionStorage.removeItem(FLAG_KEY);
    else sessionStorage.setItem(FLAG_KEY, value);
  } catch {
    // memoryFlag already holds it
  }
}

/** Entry-script URL identifies the build, so a flag from an older build never suppresses recovery. */
function buildId(): string {
  const src = document.querySelector<HTMLScriptElement>('script[src*="/_next/static/"]')?.src ?? "unknown";
  let h = 0;
  for (let i = 0; i < src.length; i++) h = (Math.imul(31, h) + src.charCodeAt(i)) | 0;
  return String(h >>> 0);
}

// Dirty tracking. Controlled inputs keep defaultValue in step with value, so it is never consulted.
const dirty = new Set<WeakRef<Element>>();
const refs = new WeakMap<Element, WeakRef<Element>>();
let tracking = false;

function untrack(el: Element): void {
  const ref = refs.get(el);
  if (ref) {
    dirty.delete(ref);
    refs.delete(el);
  }
}

function fieldValue(el: Element): string {
  return (el as HTMLInputElement).value ?? el.textContent ?? "";
}

function onInput(e: Event): void {
  const el = e.target;
  if (!(el instanceof Element) || el.matches(EXCLUDED)) return;
  if (fieldValue(el) === "") return untrack(el);
  if (refs.has(el)) return;
  const ref = new WeakRef(el);
  refs.set(el, ref);
  dirty.add(ref);
}

function onSubmit(e: Event): void {
  const form = e.target;
  if (!(form instanceof HTMLFormElement)) return;
  for (const ref of [...dirty]) {
    const el = ref.deref();
    if (el && form.contains(el)) untrack(el);
  }
}

function installDirtyTracking(): void {
  if (tracking || typeof document === "undefined") return;
  tracking = true;
  document.addEventListener("input", onInput, true);
  document.addEventListener("submit", onSubmit, true);
}

function pruneDirty(): void {
  for (const ref of [...dirty]) {
    const el = ref.deref();
    if (!el || !el.isConnected) dirty.delete(ref);
  }
}

/** Test-only: size of the pruned dirty set. */
export function dirtyCount(): number {
  pruneDirty();
  return dirty.size;
}

/** Test-only: whether the element is still reachable from the dirty set. */
export function isTracked(el: Element): boolean {
  pruneDirty();
  const ref = refs.get(el);
  return !!ref && dirty.has(ref);
}

function hasUnsavedInput(): boolean {
  pruneDirty();
  if (dirty.size > 0) return true;
  for (const el of document.querySelectorAll<HTMLElement>(TEXT_FIELDS)) {
    if (el.matches(EXCLUDED) || el.hidden || getComputedStyle(el).display === "none") continue;
    if (fieldValue(el) !== "") return true;
  }
  return false;
}

function recordEvent(kind: "reload" | "banner"): void {
  try {
    performance.mark("stale-chunk-reload", { detail: { kind } });
  } catch {
    // telemetry must never break recovery
  }
}

function showBanner(): void {
  if (document.getElementById(BANNER_ID)) return;
  const banner = document.createElement("div");
  banner.id = BANNER_ID;
  banner.setAttribute("role", "status");
  banner.style.cssText =
    "position:fixed;bottom:16px;left:16px;right:16px;z-index:2147483647;padding:12px 16px;border-radius:8px;background:#1f2937;color:#fff;display:flex;gap:12px;align-items:center;justify-content:space-between";
  const text = document.createElement("span");
  text.textContent = "A new version is available. Reload when ready.";
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = "Reload";
  button.addEventListener("click", () => page.reload());
  banner.append(text, button);
  document.body.append(banner);
}

/** Indirection so tests can observe reloads; jsdom's location.reload cannot be spied on. */
export const page = { reload: () => location.reload() };

function cancelSettle(): void {
  clearTimeout(settleTimer);
  settleTimer = undefined;
}

/** Called when a lazy import rejects. Never resolves (page reloads or banner shows) or rejects to the ErrorBoundary. */
export function recoverFromChunkError<T>(err: unknown): Promise<T> {
  if (!isChunkLoadError(err)) return Promise.reject(err);
  const id = buildId();
  if (readFlag() === id) return Promise.reject(err);
  cancelSettle();
  try {
    if (hasUnsavedInput()) {
      recordEvent("banner");
      showBanner();
    } else {
      writeFlag(id);
      recordEvent("reload");
      page.reload();
    }
  } catch (recoveryError) {
    return Promise.reject(recoveryError);
  }
  return new Promise<T>(() => {});
}

/** Clears the reload flag once a chunk has loaded and no failure follows within SETTLE_MS. */
export function markLoadSucceeded(): void {
  if (readFlag() === null || settleTimer !== undefined) return;
  settleTimer = setTimeout(() => {
    settleTimer = undefined;
    writeFlag(null);
  }, SETTLE_MS);
}

installDirtyTracking();

/** Test-only reset. */
export function resetStaleChunkForTests(): void {
  cancelSettle();
  memoryFlag = null;
  dirty.clear();
  try {
    sessionStorage.removeItem(FLAG_KEY);
  } catch {
    // ignore
  }
}
