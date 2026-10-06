// logPatterns.test.ts — unit tests for Datadog-style log pattern clustering.

import { normalizeMessage, groupLogsByPattern } from "../logPatterns";
import type { LogEntry } from "@/lib/hooks/useLogViewer";

function makeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    id: `id-${Math.random()}`,
    timestamp: "2026-08-25T00:00:00.000Z",
    level: "WARN",
    message: "worktree directory missing, marking as paused path=/tmp/wt-1",
    raw: "worktree directory missing, marking as paused path=/tmp/wt-1",
    ...overrides,
  };
}

describe("normalizeMessage", () => {
  it("replaces UUIDs with a placeholder", () => {
    const msg = "session bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb attached";
    expect(normalizeMessage(msg)).toBe("session <uuid> attached");
  });

  it("replaces multi-segment filesystem paths with a placeholder", () => {
    const msg = "worktree directory missing path=/Users/tstapler/.stapler-squad/workspaces/abc";
    expect(normalizeMessage(msg)).toContain("<path>");
    expect(normalizeMessage(msg)).not.toContain("/Users/tstapler");
  });

  it("replaces embedded JSON blobs with a placeholder", () => {
    const msg = 'Terminal stream error (managed): {"code":1,"reason":"eof"}';
    expect(normalizeMessage(msg)).toBe("Terminal stream error (managed): <obj>");
  });

  it("replaces bare numbers with a placeholder", () => {
    expect(normalizeMessage("dropped 42 items")).toBe("dropped <n> items");
  });

  it("leaves a message with no dynamic content unchanged", () => {
    const msg = "tmux session doesn't exist, no need to kill";
    expect(normalizeMessage(msg)).toBe(msg);
  });
});

describe("groupLogsByPattern", () => {
  it("groups messages that normalize to the same pattern under one entry", () => {
    const entries = [
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-1" }),
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-2" }),
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-3" }),
    ];

    const groups = groupLogsByPattern(entries);

    expect(groups).toHaveLength(1);
    expect(groups[0].count).toBe(3);
    expect(groups[0].pattern).toBe("worktree directory missing, marking as paused path=<path>");
    expect(groups[0].entries).toHaveLength(3);
  });

  it("builds a stable key from level and pattern joined by a literal space", () => {
    const groups = groupLogsByPattern([makeEntry({ level: "WARN", message: "x" })]);
    expect(groups[0].key).toBe("WARN x");
  });

  it("keeps different levels separate even for the same message text", () => {
    const entries = [
      makeEntry({ level: "WARN", message: "flaky thing happened" }),
      makeEntry({ level: "ERROR", message: "flaky thing happened" }),
    ];

    const groups = groupLogsByPattern(entries);

    expect(groups).toHaveLength(2);
  });

  it("sorts groups by count descending", () => {
    const entries = [
      makeEntry({ message: "rare event" }),
      makeEntry({ message: "common event" }),
      makeEntry({ message: "common event" }),
      makeEntry({ message: "common event" }),
    ];

    const groups = groupLogsByPattern(entries);

    expect(groups[0].pattern).toBe("common event");
    expect(groups[0].count).toBe(3);
    expect(groups[1].pattern).toBe("rare event");
    expect(groups[1].count).toBe(1);
  });

  it("preserves the raw, non-normalized message on each entry for expansion", () => {
    const entries = [makeEntry({ message: "queue full, dropping path=/tmp/a total_dropped=100" })];

    const groups = groupLogsByPattern(entries);

    expect(groups[0].entries[0].message).toBe("queue full, dropping path=/tmp/a total_dropped=100");
    expect(groups[0].pattern).toBe("queue full, dropping path=<path> total_dropped=<n>");
  });

  it("returns an empty array for no entries", () => {
    expect(groupLogsByPattern([])).toEqual([]);
  });
});
