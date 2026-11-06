import { execFile } from 'child_process';
import { promisify } from 'util';
import { SessionClient } from '../helpers/session-client';

const execFileAsync = promisify(execFile);

/** Single definition of the perf fixture shape (Story 1.1.3): w1 = 8 session panes, w2-w4 = 4 each (plus one session-list pane per window). */
export const perfFixtureShape = {
  panesPerWindow: [8, 4, 4, 4],
  backlogItems: 200,
} as const;

export const perfSessionCount = perfFixtureShape.panesPerWindow.reduce((a, b) => a + b, 0);

export const WINDOW_LAYOUT_KEY = 'cockpit.windowLayout';
const LAST_FOCUSED_WINDOW_KEY = 'cockpit.lastFocusedWindowId';

interface Leaf {
  type: 'leaf';
  id: string;
  viewKind: 'session-detail' | 'session-list';
  sessionId: string | null;
  activeTab: 'terminal';
}
interface Split {
  type: 'split';
  id: string;
  direction: 'vertical' | 'horizontal';
  ratio: number;
  first: Node;
  second: Node;
}
type Node = Leaf | Split;

function leaf(windowIdx: number, paneIdx: number, sessionId: string): Leaf {
  return { type: 'leaf', id: `perf-w${windowIdx}-p${paneIdx}`, viewKind: 'session-detail', sessionId, activeTab: 'terminal' };
}

function listLeaf(windowIdx: number): Leaf {
  return { type: 'leaf', id: `perf-w${windowIdx}-list`, viewKind: 'session-list', sessionId: null, activeTab: 'terminal' };
}

/** Balanced binary split tree over `leaves`, alternating direction with depth. */
function tree(windowIdx: number, leaves: Leaf[], depth = 0): Node {
  if (leaves.length === 1) return leaves[0];
  const mid = Math.ceil(leaves.length / 2);
  return {
    type: 'split',
    id: `perf-w${windowIdx}-s${depth}-${leaves[0].id}`,
    direction: depth % 2 === 0 ? 'vertical' : 'horizontal',
    ratio: mid / leaves.length,
    first: tree(windowIdx, leaves.slice(0, mid), depth + 1),
    second: tree(windowIdx, leaves.slice(mid), depth + 1),
  };
}

/** Persisted v2 window layout (`cockpit.windowLayout`) assigning `sessionIds` across the fixture windows in order. */
export function buildWindowLayout(sessionIds: string[]) {
  let cursor = 0;
  const windows = perfFixtureShape.panesPerWindow.map((count, w) => {
    const leaves = Array.from({ length: count }, (_, p) => leaf(w, p, sessionIds[cursor++]));
    return {
      id: `perf-window-${w + 1}`,
      name: `Window ${w + 1}`,
      // useWindowManager.repairWindow resets any window lacking a session-list leaf, so each window gets one beside its session panes.
      paneState: {
        root: { type: 'split' as const, id: `perf-w${w}-root`, direction: 'vertical' as const, ratio: 0.15, first: listLeaf(w), second: tree(w, leaves) },
        focusedPaneId: leaves[0].id,
        zoomedPaneId: null,
      },
    };
  });
  return { version: 2 as const, revision: 1, windows };
}

export interface SeededFixture {
  sessionTitles: string[];
  sessionIds: string[];
  layout: ReturnType<typeof buildWindowLayout>;
  /** Wall-clock seeding cost, recorded by the Task 1.1.3e CI feasibility spike. */
  seedMs: { sessions: number; backlog: number };
}

async function rpcPost(baseUrl: string, method: string, data: unknown): Promise<void> {
  const resp = await fetch(`${baseUrl}/api/session.v1.SessionService/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!resp.ok) throw new Error(`${method} failed: HTTP ${resp.status} ${await resp.text().catch(() => '')}`);
}

/**
 * Seeds `perfFixtureShape` against an isolated instance: sessions via CreateSession (real tmux, `bash`),
 * backlog items via the e2e-local debug mutate endpoint, backlog flag on. Idempotent on titles.
 * `sessionCount`/`backlogCount` shrink the fixture for the CI-fallback case; the actual sizes land in baseline metadata.
 */
export async function seedPerfFixture(
  baseUrl: string,
  opts: { sessionCount?: number; backlogCount?: number; path?: string } = {}
): Promise<SeededFixture> {
  const path = opts.path ?? '/tmp';
  const client = new SessionClient(baseUrl);
  const existing = new Map((await client.listSessions()).map((s) => [s.title, s]));
  const titles = Array.from({ length: perfSessionCount }, (_, i) => `perf-session-${String(i + 1).padStart(2, '0')}`);
  const wanted = titles.slice(0, opts.sessionCount ?? perfSessionCount);

  const t0 = Date.now();
  for (const title of wanted) {
    if (!existing.has(title)) existing.set(title, await client.createSession({ title, path, program: 'bash' }));
  }
  const sessionsMs = Date.now() - t0;

  const t1 = Date.now();
  await rpcPost(baseUrl, 'UpdateFeatureFlag', { name: 'backlog', enabled: true });
  const backlogCount = opts.backlogCount ?? perfFixtureShape.backlogItems;
  for (let i = 0; i < backlogCount; i++) {
    const resp = await fetch(`${baseUrl}/api/debug/backlog/mutate-create`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: `perf-backlog-${String(i + 1).padStart(3, '0')}`, description: '', status: 'idea', priority: 3, repoPath: '', acCriteria: [] }),
    });
    if (!resp.ok) throw new Error(`mutate-create failed: HTTP ${resp.status}`);
  }
  const backlogMs = Date.now() - t1;

  // Fewer sessions than panes (fallback fixture): reuse sessions round-robin so the pane count stays fixed.
  const ids = Array.from({ length: perfSessionCount }, (_, i) => existing.get(wanted[i % wanted.length])!.id);
  return { sessionTitles: wanted, sessionIds: ids, layout: buildWindowLayout(ids), seedMs: { sessions: sessionsMs, backlog: backlogMs } };
}

/** Storage that every perf page load starts from: opt-in gauge, onboarding suppressed, fixture windows (only if none persisted yet). */
export function perfInitScript(layout: unknown): string {
  return `(() => { try {
    localStorage.setItem('ssq:perf', '1');
    for (const k of ['onboarding','tour','hint','welcome']) localStorage.setItem('ssq-'+k+'-complete','true');
    if (!localStorage.getItem(${JSON.stringify(WINDOW_LAYOUT_KEY)})) {
      localStorage.setItem(${JSON.stringify(WINDOW_LAYOUT_KEY)}, ${JSON.stringify(JSON.stringify(layout))});
      localStorage.setItem(${JSON.stringify(LAST_FOCUSED_WINDOW_KEY)}, 'perf-window-1');
    }
  } catch (e) {} })();`;
}

/** `<cols>x<rows>` of `sessionName`'s pane on the instance's private tmux socket (used by Task 4.1.4c). */
export async function tmuxPaneDims(sessionName: string, socket = process.env.STAPLER_SQUAD_TMUX_SOCKET ?? ''): Promise<string> {
  const args = ['display-message', '-p', '-t', sessionName, '#{pane_width}x#{pane_height}'];
  const { stdout } = await execFileAsync('tmux', socket ? ['-L', socket, ...args] : args);
  return stdout.trim();
}

const LIVE_INSTANCE_PORTS = ['8543', '8444'];

/** Refuses to seed or drive the live deployed instance (REQ-15); perf runs only against the isolated global-setup server. */
export function assertNotLiveInstance(baseUrl: string): void {
  const port = new URL(baseUrl).port;
  if (LIVE_INSTANCE_PORTS.includes(port)) {
    throw new Error(`perf spec refuses to run against the live instance (${baseUrl}); use the isolated global-setup server`);
  }
}
