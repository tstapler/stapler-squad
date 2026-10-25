// Browser-side perf snapshot for "the UI is slow with many windows/panes" reports.
// Exports a whitelisted layout (pane ids, split geometry, view kind, tab; window names and session
// ids are dropped because a legacy session id is its title),
// DOM/heap/long-task stats and the user agent. No terminal content.

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";

interface LayoutNode {
  type: "leaf" | "split";
  id?: string;
  direction?: string;
  ratio?: number;
  sessionId?: string | null;
  viewKind?: string;
  activeTab?: string;
  first?: LayoutNode;
  second?: LayoutNode;
}

export interface PerfDiagnostics {
  capturedAt: string;
  userAgent: string;
  viewport: { width: number; height: number };
  dom: { nodes: number; terminals: number };
  heapUsedMB: number | null;
  windows: { id: string; panes: number; terminalPanes: number; activeTabs: Record<string, number> }[];
  longTasks: { count: number; maxMs: number; totalMs: number };
  layout: unknown;
}

function countLeaves(node: LayoutNode | undefined, acc: { panes: number; terminalPanes: number; activeTabs: Record<string, number> }) {
  if (!node) return;
  if (node.type === "leaf") {
    acc.panes++;
    if (node.viewKind === "session-detail") {
      acc.terminalPanes++;
      const tab = node.activeTab ?? "unknown";
      acc.activeTabs[tab] = (acc.activeTabs[tab] ?? 0) + 1;
    }
    return;
  }
  countLeaves(node.first, acc);
  countLeaves(node.second, acc);
}

function sanitizeNode(node: LayoutNode | undefined): unknown {
  if (!node) return null;
  if (node.type === "leaf") {
    return { type: "leaf", id: node.id, viewKind: node.viewKind, hasSession: node.sessionId != null, activeTab: node.activeTab };
  }
  return { type: "split", id: node.id, direction: node.direction, ratio: node.ratio, first: sanitizeNode(node.first), second: sanitizeNode(node.second) };
}

function readLayout(): unknown {
  try {
    const stored = localStorage.getItem(WINDOW_LAYOUT_KEY);
    return stored ? JSON.parse(stored) : null;
  } catch {
    return null;
  }
}

/** Resolves buffered longtask entries (the browser keeps the most recent ~200). */
function collectLongTasks(): Promise<PerfDiagnostics["longTasks"]> {
  const empty = { count: 0, maxMs: 0, totalMs: 0 };
  if (typeof PerformanceObserver === "undefined" || !PerformanceObserver.supportedEntryTypes?.includes("longtask")) {
    return Promise.resolve(empty);
  }
  return new Promise((resolve) => {
    const durations: number[] = [];
    const observer = new PerformanceObserver((list) => {
      for (const e of list.getEntries()) durations.push(e.duration);
    });
    observer.observe({ type: "longtask", buffered: true });
    // Buffered entries are delivered to the callback asynchronously; wait briefly, then drain any still queued.
    setTimeout(() => {
      for (const e of observer.takeRecords()) durations.push(e.duration);
      observer.disconnect();
      resolve({
        count: durations.length,
        maxMs: Math.round(Math.max(0, ...durations)),
        totalMs: Math.round(durations.reduce((a, b) => a + b, 0)),
      });
    }, 50);
  });
}

export async function collectPerfDiagnostics(): Promise<PerfDiagnostics> {
  const rawLayout = readLayout() as { windows?: { id: string; paneState?: { root?: LayoutNode; focusedPaneId?: string; zoomedPaneId?: string | null } }[] } | null;
  const rawWindows = Array.isArray(rawLayout?.windows) ? rawLayout.windows : [];
  const windows = rawWindows.map((w) => {
    const acc = { panes: 0, terminalPanes: 0, activeTabs: {} as Record<string, number> };
    countLeaves(w.paneState?.root, acc);
    return { id: w.id, ...acc };
  });
  const memory = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory;
  return {
    capturedAt: new Date().toISOString(),
    userAgent: navigator.userAgent,
    viewport: { width: window.innerWidth, height: window.innerHeight },
    dom: { nodes: document.getElementsByTagName("*").length, terminals: document.querySelectorAll(".xterm").length },
    heapUsedMB: memory ? Math.round(memory.usedJSHeapSize / 1048576) : null,
    windows,
    longTasks: await collectLongTasks(),
    layout: rawWindows.map((w) => ({
      id: w.id,
      focusedPaneId: w.paneState?.focusedPaneId,
      zoomedPaneId: w.paneState?.zoomedPaneId,
      root: sanitizeNode(w.paneState?.root),
    })),
  };
}
