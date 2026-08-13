/**
 * Tests for MessageQueue - async-iterable queue for outgoing terminal messages.
 *
 * Uses mock data objects instead of real protobuf types to avoid the
 * pre-existing Jest module resolution issue with generated `.js` imports.
 */

import { MessageQueue } from '../MessageQueue';

// Mock the events_pb module to avoid protobuf resolution issues in Jest.
// The real TerminalData is a plain protobuf v2 object with sessionId and data fields.
jest.mock('@/gen/session/v1/events_pb', () => {
  return {
    TerminalData: class {},
    TerminalDataSchema: Symbol('MockTerminalDataSchema'),
    TerminalInput: class {},
  };
});

// Mock @bufbuild/protobuf so that create(schema, init) returns a plain object
jest.mock('@bufbuild/protobuf', () => ({
  create: (_schema: unknown, init: Record<string, unknown>) => ({ ...init }),
}));

function createTestMessage(sessionId: string, input?: string): any {
  return {
    sessionId,
    data: input ? { case: "input" as const, value: { data: input } } : { case: undefined },
  };
}

describe('MessageQueue', () => {
  describe('push and iterate', () => {
    it('should yield messages in order', async () => {
      const queue = new MessageQueue();
      const msg1 = createTestMessage('s1', 'hello');
      const msg2 = createTestMessage('s1', 'world');

      queue.push(msg1);
      queue.push(msg2);
      queue.close();

      const received: any[] = [];
      for await (const msg of queue) {
        received.push(msg);
      }

      expect(received).toHaveLength(2);
      expect(received[0].sessionId).toBe('s1');
      expect(received[1].sessionId).toBe('s1');
    });

    it('should yield messages pushed while iterating', async () => {
      const queue = new MessageQueue();

      const received: any[] = [];
      const iterPromise = (async () => {
        for await (const msg of queue) {
          received.push(msg);
        }
      })();

      queue.push(createTestMessage('s1', 'first'));
      await new Promise(r => setTimeout(r, 10));
      queue.push(createTestMessage('s1', 'second'));
      await new Promise(r => setTimeout(r, 10));
      queue.close();

      await iterPromise;

      expect(received).toHaveLength(2);
    });
  });

  describe('close', () => {
    it('should unblock a waiting iterator', async () => {
      const queue = new MessageQueue();

      const received: any[] = [];
      const iterPromise = (async () => {
        for await (const msg of queue) {
          received.push(msg);
        }
      })();

      queue.close();
      await iterPromise;

      expect(received).toHaveLength(0);
    });

    it('should filter out sentinel messages', async () => {
      const queue = new MessageQueue();

      queue.push(createTestMessage('s1', 'real'));
      queue.close();

      const received: any[] = [];
      for await (const msg of queue) {
        received.push(msg);
      }

      expect(received).toHaveLength(1);
      expect(received[0].sessionId).toBe('s1');
    });
  });

  describe('push after close', () => {
    it('should be a no-op', async () => {
      const queue = new MessageQueue();
      queue.close();

      queue.push(createTestMessage('s1', 'late'));

      const received: any[] = [];
      for await (const msg of queue) {
        received.push(msg);
      }

      expect(received).toHaveLength(0);
    });
  });

  describe('isClosed', () => {
    it('should return false initially', () => {
      const queue = new MessageQueue();
      expect(queue.isClosed()).toBe(false);
    });

    it('should return true after close', () => {
      const queue = new MessageQueue();
      queue.close();
      expect(queue.isClosed()).toBe(true);
    });
  });
});
