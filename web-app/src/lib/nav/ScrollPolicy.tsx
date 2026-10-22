import { useContext, useEffect, useLayoutEffect, useRef } from "react";
import { UNSAFE_DataRouterStateContext, useLocation, useNavigationType } from "react-router";
import { normalizePathname } from "./normalize";

// `<ScrollRestoration>` only manages `window`, which never scrolls in this app: each page scrolls an inner element
// (gate S4, project_plans/web-app-bun-build/research/spike-results.md), so scroll is reset and restored here.

const WINDOW_KEY = "window";
const MAX_REMEMBERED_LOCATIONS = 100;
const RESTORE_WINDOW_MS = 1500;
const RESTORE_RETRY_MS = 50;
const TOLERANCE_PX = 2;

interface Position {
  top: number;
  left: number;
}

const positionsByLocation = new Map<string, Map<string, Position>>();

/** Stable across remounts of the same page: an id, else the tag/sibling-index path from `body`. */
function describe(element: Element): string {
  if (element.id) return `#${element.id}`;
  const segments: string[] = [];
  for (let node: Element | null = element; node && node !== document.body; node = node.parentElement) {
    const siblings = node.parentElement ? Array.from(node.parentElement.children) : [];
    segments.push(`${node.tagName.toLowerCase()}:${siblings.indexOf(node)}`);
  }
  return segments.reverse().join(">");
}

function find(descriptor: string): Element | null {
  if (descriptor.startsWith("#")) return document.getElementById(descriptor.slice(1));
  let node: Element | null = document.body;
  for (const segment of descriptor.split(">")) {
    const index = Number(segment.slice(segment.lastIndexOf(":") + 1));
    node = node?.children[index] ?? null;
  }
  return node;
}

function remember(locationKey: string, descriptor: string, position: Position): void {
  const forLocation = positionsByLocation.get(locationKey) ?? new Map<string, Position>();
  forLocation.set(descriptor, position);
  positionsByLocation.delete(locationKey);
  positionsByLocation.set(locationKey, forLocation);
  const oldest = positionsByLocation.keys().next().value;
  if (positionsByLocation.size > MAX_REMEMBERED_LOCATIONS && oldest !== undefined) positionsByLocation.delete(oldest);
}

function applyPosition(descriptor: string, position: Position): boolean {
  if (descriptor === WINDOW_KEY) {
    window.scrollTo(position.left, position.top);
    return Math.abs(window.scrollY - position.top) <= TOLERANCE_PX;
  }
  const element = find(descriptor);
  if (!element) return false;
  element.scrollTop = position.top;
  element.scrollLeft = position.left;
  return Math.abs(element.scrollTop - position.top) <= TOLERANCE_PX;
}

/** Retries while a lazy page and its data grow the scroller; stops on success, user input, or timeout. */
function restore(saved: Map<string, Position>): () => void {
  const pending = new Map(saved);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const startedAt = Date.now();
  const stop = () => {
    clearTimeout(timer);
    for (const type of ["wheel", "touchstart", "keydown"]) window.removeEventListener(type, stop, true);
  };
  const attempt = () => {
    for (const [descriptor, position] of pending) if (applyPosition(descriptor, position)) pending.delete(descriptor);
    if (pending.size === 0 || Date.now() - startedAt > RESTORE_WINDOW_MS) return stop();
    timer = setTimeout(attempt, RESTORE_RETRY_MS);
  };
  for (const type of ["wheel", "touchstart", "keydown"]) window.addEventListener(type, stop, { capture: true, passive: true });
  attempt();
  return stop;
}

/** Forward navigation to a new pathname scrolls to the top, Back restores, and a search-only change keeps position. */
export function ScrollPolicy(): null {
  const location = useLocation();
  const navigationType = useNavigationType();
  const preventScrollReset = useContext(UNSAFE_DataRouterStateContext)?.preventScrollReset === true;
  const currentKey = useRef(location.key);
  const previousPathname = useRef(normalizePathname(location.pathname));
  const scrollers = useRef(new Set<Element>());

  useEffect(() => {
    const record = (event: Event) => {
      const target = event.target;
      if (target === document) return remember(currentKey.current, WINDOW_KEY, { top: window.scrollY, left: window.scrollX });
      if (!(target instanceof Element)) return;
      scrollers.current.add(target);
      remember(currentKey.current, describe(target), { top: target.scrollTop, left: target.scrollLeft });
    };
    document.addEventListener("scroll", record, { capture: true, passive: true });
    return () => document.removeEventListener("scroll", record, true);
  }, []);

  useLayoutEffect(() => {
    currentKey.current = location.key;
    const pathname = normalizePathname(location.pathname);
    if (pathname === previousPathname.current) return;
    previousPathname.current = pathname;
    if (navigationType === "POP") {
      const saved = positionsByLocation.get(location.key);
      return saved ? restore(saved) : undefined;
    }
    if (preventScrollReset) return;
    window.scrollTo(0, 0);
    scrollers.current.forEach((element) => {
      if (!element.isConnected) scrollers.current.delete(element);
      else {
        element.scrollTop = 0;
        element.scrollLeft = 0;
      }
    });
  }, [location.key, location.pathname, navigationType, preventScrollReset]);

  return null;
}
