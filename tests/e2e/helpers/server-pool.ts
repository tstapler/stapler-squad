import { cleanupSharedAssets, prepareSharedAssets, SharedAssets, TestServer } from './test-server';

export interface ServerPool {
  servers: TestServer[];
  assets: SharedAssets;
}

let pool: ServerPool | null = null;

/** Per-slot data dir: the single-server path keeps today's exact name. */
function slotTestDir(base: string, slot: number, size: number): string {
  return size === 1 ? base : `${base}-w${slot}`;
}

/**
 * Starts `size` isolated servers concurrently, one per Playwright worker slot. Each gets its own
 * port, --test-dir (and so its own instance.lock, DB and logs) and private tmux socket.
 */
export async function startServerPool(size: number): Promise<ServerPool> {
  const assets = await prepareSharedAssets();
  const base = process.env.TEST_SERVER_DIR || `/tmp/stapler-squad-test-${process.pid}`;
  const servers = Array.from({ length: size }, (_, slot) => new TestServer({ assets, testDir: slotTestDir(base, slot, size) }));
  try {
    await Promise.all(servers.map((s) => s.start()));
  } catch (err) {
    await Promise.allSettled(servers.map((s) => s.stop()));
    await cleanupSharedAssets(assets);
    throw err;
  }
  pool = { servers, assets };
  return pool;
}

export function getServerPool(): ServerPool {
  if (!pool) throw new Error('server pool not started');
  return pool;
}

export async function stopServerPool(): Promise<void> {
  if (!pool) return;
  const { servers, assets } = pool;
  pool = null;
  await Promise.allSettled(servers.map((s) => s.stop()));
  await cleanupSharedAssets(assets);
}
