/**
 * Typed ConnectRPC JSON client for integration tests.
 * Uses the ConnectRPC JSON wire format (POST /api/session.v1.SessionService/<Method>)
 * so tests can create, modify, and inspect server state without raw fetch() calls.
 *
 * Usage:
 *   import { SessionClient } from './helpers/session-client';
 *   const client = new SessionClient(process.env.TEST_SERVER_URL!);
 *   const session = await client.createSession({ title: 'my-test', path: '/tmp' });
 *   await client.waitForReviewQueue(1);
 */

// Proto enum values (session/v1/types.proto)
export const SessionStatus = {
  Unspecified: 0,
  Running: 1,
  Ready: 2,
  Loading: 3,
  Paused: 4,
  NeedsApproval: 5,
  Creating: 6,
  Stopped: 7,
} as const;

export interface CreateSessionOptions {
  title: string;
  path?: string;
  workingDir?: string;
  branch?: string;
  program?: string;
  category?: string;
  tags?: string[];
  oneOff?: boolean;
  prompt?: string;
  autoYes?: boolean;
}

export interface Session {
  id: string;
  title: string;
  status: string;
  path: string;
  program: string;
  category: string;
  tags: string[];
  branch: string;
  workingDir: string;
}

export interface ReviewItem {
  sessionId: string;
  sessionName: string;
  reason: number;
  priority: number;
  context: string;
  program: string;
  path: string;
  branch: string;
  tags: string[];
}

export interface ReviewQueue {
  totalItems: number;
  items: ReviewItem[];
}

class RpcError extends Error {
  constructor(
    public readonly method: string,
    public readonly statusCode: number,
    public readonly body: string,
  ) {
    super(`${method} failed: HTTP ${statusCode} ${body}`);
    this.name = 'RpcError';
  }

  get isAlreadyExists(): boolean {
    return this.statusCode === 409;
  }

  get isNotFound(): boolean {
    return this.statusCode === 404;
  }
}

export class SessionClient {
  constructor(private readonly baseUrl: string) {}

  private async rpc<T>(method: string, body: Record<string, unknown>): Promise<T> {
    const resp = await fetch(`${this.baseUrl}/api/session.v1.SessionService/${method}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const text = await resp.text().catch(() => '');
      throw new RpcError(method, resp.status, text);
    }
    return resp.json() as Promise<T>;
  }

  async createSession(opts: CreateSessionOptions): Promise<Session> {
    const resp = await this.rpc<{ session: Session }>('CreateSession', {
      title: opts.title,
      path: opts.path ?? '',
      workingDir: opts.workingDir,
      branch: opts.branch,
      program: opts.program ?? 'bash',
      category: opts.category,
      tags: opts.tags,
      oneOff: opts.oneOff,
      prompt: opts.prompt,
      autoYes: opts.autoYes,
    });
    return resp.session;
  }

  async listSessions(): Promise<Session[]> {
    const resp = await this.rpc<{ sessions: Session[] }>('ListSessions', {});
    return resp.sessions ?? [];
  }

  async getSession(id: string): Promise<Session> {
    const resp = await this.rpc<{ session: Session }>('GetSession', { id });
    return resp.session;
  }

  async updateSession(id: string, updates: {
    status?: number;
    title?: string;
    category?: string;
    tags?: string[];
  }): Promise<Session> {
    const resp = await this.rpc<{ session: Session }>('UpdateSession', { id, ...updates });
    return resp.session;
  }

  async pauseSession(id: string): Promise<Session> {
    return this.updateSession(id, { status: SessionStatus.Paused });
  }

  async resumeSession(id: string): Promise<Session> {
    return this.updateSession(id, { status: SessionStatus.Running });
  }

  async deleteSession(id: string, force = false): Promise<void> {
    await this.rpc('DeleteSession', { id, force });
  }

  async acknowledgeSession(id: string): Promise<void> {
    await this.rpc('AcknowledgeSession', { id });
  }

  async getReviewQueue(): Promise<ReviewQueue> {
    const resp = await this.rpc<{ reviewQueue: ReviewQueue }>('GetReviewQueue', {});
    return resp.reviewQueue ?? { totalItems: 0, items: [] };
  }

  /**
   * Poll the review queue until at least `minItems` items appear.
   * Useful after creating sessions that need time to become idle.
   */
  async waitForReviewQueue(minItems = 1, timeoutMs = 15000): Promise<ReviewQueue> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const queue = await this.getReviewQueue();
      if (queue.totalItems >= minItems) return queue;
      await new Promise(r => setTimeout(r, 1000));
    }
    const queue = await this.getReviewQueue();
    throw new Error(
      `Review queue has ${queue.totalItems} items after ${timeoutMs}ms, wanted >= ${minItems}`
    );
  }

  /**
   * Create a session that will appear in the review queue once it goes idle.
   * Returns the created session; call waitForReviewQueue() to confirm it landed.
   */
  async createIdleSession(
    title: string,
    sessionPath: string,
    extras: Partial<CreateSessionOptions> = {},
  ): Promise<Session> {
    return this.createSession({ title, path: sessionPath, program: 'bash', ...extras });
  }
}

export function createSessionClient(baseUrl: string): SessionClient {
  return new SessionClient(baseUrl);
}

export { RpcError };
