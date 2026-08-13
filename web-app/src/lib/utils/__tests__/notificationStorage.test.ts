/**
 * Tests for notificationStorage.ts — localStorage-backed deduplication.
 *
 * Covers:
 *  1. shouldNotify: unknown / within-TTL / expired-TTL / within-grace / expired-grace
 *  2. markNotifiedBatch: bulk marking
 *  3. cleanupExpired: removes records older than TTL
 *  4. markAcknowledged: preserves original notifiedAt timestamp
 *  5. getAcknowledgedSessions: returns sessions currently within grace period
 */

import {
  shouldNotify,
  markNotified,
  markAcknowledged,
  markNotifiedBatch,
  cleanupExpired,
  getNotifiedSessions,
  getAcknowledgedSessions,
  clearAll,
} from "../notificationStorage";

// These match the private constants in notificationStorage.ts
const NOTIFICATION_TTL_MS = 60 * 60 * 1000; // 1 hour
const GRACE_PERIOD_MS = 5 * 60 * 1000;       // 5 minutes

const BASE_TIME = new Date("2024-01-01T00:00:00Z").getTime();

beforeEach(() => {
  jest.useFakeTimers();
  jest.setSystemTime(BASE_TIME);
  clearAll();
});

afterEach(() => {
  jest.useRealTimers();
  clearAll();
});

// ── 1. shouldNotify ──────────────────────────────────────────────────────────

describe("shouldNotify", () => {
  it("returns true for a session that has never been seen", () => {
    expect(shouldNotify("unknown-session")).toBe(true);
  });

  it("returns false immediately after markNotified (within TTL)", () => {
    markNotified("session-a");
    expect(shouldNotify("session-a")).toBe(false);
  });

  it("returns true once the TTL expires (> 1 hour)", () => {
    markNotified("session-ttl");
    jest.advanceTimersByTime(NOTIFICATION_TTL_MS + 1);
    expect(shouldNotify("session-ttl")).toBe(true);
  });

  it("returns false at exactly the TTL boundary (not yet expired)", () => {
    markNotified("session-boundary");
    jest.advanceTimersByTime(NOTIFICATION_TTL_MS);
    // At exactly TTL, it is NOT expired (> TTL required)
    expect(shouldNotify("session-boundary")).toBe(false);
  });

  it("returns false within the acknowledgment grace period", () => {
    markNotified("session-ack");
    markAcknowledged("session-ack");
    jest.advanceTimersByTime(GRACE_PERIOD_MS - 1);
    expect(shouldNotify("session-ack")).toBe(false);
  });

  it("returns false after only grace period expires (TTL still active)", () => {
    // The grace period (5 min) gates re-notification but the TTL (1 hour) is the
    // true re-enable boundary. After the grace period but before the TTL, the
    // session is still suppressed.
    markNotified("session-grace-exp");
    markAcknowledged("session-grace-exp");
    jest.advanceTimersByTime(GRACE_PERIOD_MS + 1);
    expect(shouldNotify("session-grace-exp")).toBe(false);
  });

  it("returns true after both grace period AND TTL expire", () => {
    markNotified("session-grace-ttl-exp");
    markAcknowledged("session-grace-ttl-exp");
    jest.advanceTimersByTime(NOTIFICATION_TTL_MS + 1);
    expect(shouldNotify("session-grace-ttl-exp")).toBe(true);
  });

  it("grace period check takes precedence over TTL check", () => {
    markNotified("session-both");
    markAcknowledged("session-both");
    // Still within grace period even though TTL hasn't expired
    jest.advanceTimersByTime(GRACE_PERIOD_MS - 1000);
    expect(shouldNotify("session-both")).toBe(false);
  });
});

// ── 2. markNotifiedBatch ─────────────────────────────────────────────────────

describe("markNotifiedBatch", () => {
  it("marks multiple sessions in one call", () => {
    markNotifiedBatch(["s1", "s2", "s3"]);
    expect(shouldNotify("s1")).toBe(false);
    expect(shouldNotify("s2")).toBe(false);
    expect(shouldNotify("s3")).toBe(false);
  });

  it("is a no-op for an empty array", () => {
    markNotifiedBatch([]);
    const records = getNotifiedSessions();
    expect(records.size).toBe(0);
  });

  it("preserves existing acknowledgedAt when re-marking a notified session", () => {
    markNotified("session-preserve");
    markAcknowledged("session-preserve");
    const before = getNotifiedSessions().get("session-preserve");

    markNotifiedBatch(["session-preserve"]);
    const after = getNotifiedSessions().get("session-preserve");

    expect(after?.acknowledgedAt).toBe(before?.acknowledgedAt);
  });
});

// ── 3. cleanupExpired ────────────────────────────────────────────────────────

describe("cleanupExpired", () => {
  it("removes records older than TTL", () => {
    markNotified("old-session");
    jest.advanceTimersByTime(NOTIFICATION_TTL_MS + 1);
    cleanupExpired();
    expect(getNotifiedSessions().has("old-session")).toBe(false);
  });

  it("keeps records that are still within TTL", () => {
    markNotified("fresh-session");
    jest.advanceTimersByTime(NOTIFICATION_TTL_MS - 1000);
    cleanupExpired();
    expect(getNotifiedSessions().has("fresh-session")).toBe(true);
  });

  it("is safe to call with an empty store", () => {
    expect(() => cleanupExpired()).not.toThrow();
  });
});

// ── 4. markAcknowledged ──────────────────────────────────────────────────────

describe("markAcknowledged", () => {
  it("sets acknowledgedAt on an existing record", () => {
    markNotified("session-ack2");
    markAcknowledged("session-ack2");
    const record = getNotifiedSessions().get("session-ack2");
    expect(record?.acknowledgedAt).toBeDefined();
  });

  it("preserves the original notifiedAt timestamp", () => {
    markNotified("session-preserve-ts");
    const original = getNotifiedSessions().get("session-preserve-ts");
    jest.advanceTimersByTime(5000);
    markAcknowledged("session-preserve-ts");
    const updated = getNotifiedSessions().get("session-preserve-ts");
    expect(updated?.notifiedAt).toBe(original?.notifiedAt);
  });

  it("creates a record with acknowledgedAt when session was not previously notified", () => {
    markAcknowledged("session-ack-only");
    const record = getNotifiedSessions().get("session-ack-only");
    expect(record?.acknowledgedAt).toBeDefined();
  });
});

// ── 5. getAcknowledgedSessions ───────────────────────────────────────────────

describe("getAcknowledgedSessions", () => {
  it("returns sessions within grace period after acknowledgment", () => {
    markNotified("session-in-grace");
    markAcknowledged("session-in-grace");
    const acked = getAcknowledgedSessions();
    expect(acked.has("session-in-grace")).toBe(true);
  });

  it("excludes sessions whose grace period has expired", () => {
    markNotified("session-grace-done");
    markAcknowledged("session-grace-done");
    jest.advanceTimersByTime(GRACE_PERIOD_MS + 1);
    const acked = getAcknowledgedSessions();
    expect(acked.has("session-grace-done")).toBe(false);
  });

  it("excludes sessions that were never acknowledged", () => {
    markNotified("session-not-acked");
    const acked = getAcknowledgedSessions();
    expect(acked.has("session-not-acked")).toBe(false);
  });
});
