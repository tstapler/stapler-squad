// Browser-side perf snapshot for "the UI is slow with many windows/panes" reports.
// Contains layout structure and session ids only: no terminal content, no secrets.

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";

interface LayoutNode {
  type: "leaf" | "split";
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
  layoutRaw: unknown;
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
    const observer = new PerformanceObserver(() => {});
    observer.observe({ type: "longtask", buffered: true });
    // Buffered entries are queued asynchronously; yield once before draining.
    setTimeout(() => {
      const entries = observer.takeRecords();
      observer.disconnect();
      const durations = entries.map((e) => e.duration);
      resolve({
        count: durations.length,
        maxMs: Math.round(Math.max(0, ...durations)),
        totalMs: Math.round(durations.reduce((a, b) => a + b, 0)),
      });
    }, 0);
  });
}

export async function collectPerfDiagnostics(): Promise<PerfDiagnostics> {
  const layoutRaw = readLayout();
  const rawWindows = (layoutRaw as { windows?: { id: string; paneState?: { root?: LayoutNode } }[] } | null)?.windows ?? [];
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
    layoutRaw,
  };
}
