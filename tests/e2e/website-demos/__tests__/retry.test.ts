// @feature website-demos
import { describe, expect, test } from 'bun:test';
import { NoRetryError, RetriesExhaustedError, withRetry } from '../retry';

describe('withRetry', () => {
  test('a first-try pass reports one attempt', async () => {
    expect((await withRetry(async () => 'ok')).attempts).toBe(1);
  });

  test('a flaky scene that passes the second attempt reports attempts 2', async () => {
    let calls = 0;
    const r = await withRetry(async (n) => {
      calls++;
      if (n === 1) throw new Error('locator timeout');
      return 'recorded';
    });
    expect(r).toEqual({ value: 'recorded', attempts: 2 });
    expect(calls).toBe(2);
  });

  test('a NoRetryError is never retried', async () => {
    let calls = 0;
    await expect(
      withRetry(async () => {
        calls++;
        throw new NoRetryError('expected theme class grimdark, got clean');
      }),
    ).rejects.toThrow('got clean');
    expect(calls).toBe(1);
  });

  test('exhausting attempts carries the count', async () => {
    const err = await withRetry(async () => {
      throw new Error('boom');
    }).catch((e) => e);
    expect(err).toBeInstanceOf(RetriesExhaustedError);
    expect(err.attempts).toBe(2);
  });

  test('the between-attempts check runs once and can abort the retry', async () => {
    let between = 0;
    await expect(
      withRetry(
        async () => {
          throw new Error('x');
        },
        {
          between: async () => {
            between++;
            throw new Error('server down');
          },
        },
      ),
    ).rejects.toThrow('server down');
    expect(between).toBe(1);
  });
});
