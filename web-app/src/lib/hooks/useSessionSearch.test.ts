/**
 * Tests for useSessionSearch hook.
 *
 * Covers:
 *  T-UNIT-TS-001: Empty query returns []
 *  T-UNIT-TS-002: Title weight beats path weight
 *  T-UNIT-TS-003: Fuzzy non-prefix match ("squad" → "stapler-squad")
 *  T-UNIT-TS-004: Consecutive character fuzzy match ("myfeat" → "my-feature-branch")
 *  T-UNIT-TS-005: UNSPECIFIED sessions excluded from results
 *  T-UNIT-TS-006: Results capped at 8
 *  T-UNIT-TS-007: Title+branch match scores better than title-only match
 */

import React from "react";
import { renderHook } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import reviewQueueReducer from "@/lib/store/reviewQueueSlice";
import sessionsReducer, { setSessions } from "@/lib/store/sessionsSlice";
import { useSessionSearch } from "./useSessionSearch";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStore(sessions: Session[] = []) {
  const store = configureStore({
    reducer: {
      reviewQueue: reviewQueueReducer,
      sessions: sessionsReducer,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
  if (sessions.length > 0) {
    store.dispatch(setSessions(sessions));
  }
  return store;
}

function makeSession(overrides: Partial<Parameters<typeof create>[1]> = {}): Session {
  return create(SessionSchema, {
    id: `session-${Math.random().toString(36).slice(2)}`,
    status: SessionStatus.RUNNING,
    tags: [],
    ...overrides,
  });
}

function renderWithStore(query: string, sessions: Session[]) {
  const store = makeStore(sessions);
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return renderHook(() => useSessionSearch(query), { wrapper: Wrapper });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useSessionSearch", () => {
  // T-UNIT-TS-001
  it("returns [] for empty query", () => {
    const sessions = [makeSession({ title: "auth-service" })];
    const { result } = renderWithStore("", sessions);
    expect(result.current).toEqual([]);
  });

  it("returns [] for whitespace-only query", () => {
    const sessions = [makeSession({ title: "auth-service" })];
    const { result } = renderWithStore("   ", sessions);
    expect(result.current).toEqual([]);
  });

  // T-UNIT-TS-002
  it("title match ranks above path-only match for the same query", () => {
    const sessions = [
      makeSession({
        id: "path-match",
        title: "other-thing",
        path: "/users/home/auth-stuff",
      }),
      makeSession({
        id: "title-match",
        title: "auth-service",
        path: "/users/home/other",
      }),
    ];
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBeGreaterThanOrEqual(1);
    expect(result.current[0].session.id).toBe("title-match");
  });

  // T-UNIT-TS-003
  it("fuzzy non-prefix match: 'squad' finds session with title 'stapler-squad'", () => {
    const sessions = [makeSession({ title: "stapler-squad" })];
    const { result } = renderWithStore("squad", sessions);
    expect(result.current.length).toBe(1);
    expect(result.current[0].session.title).toBe("stapler-squad");
  });

  // T-UNIT-TS-004
  it("consecutive character fuzzy match: 'myfeat' matches branch 'my-feature-branch'", () => {
    const sessions = [
      makeSession({
        title: "unrelated-session",
        branch: "my-feature-branch",
      }),
    ];
    const { result } = renderWithStore("myfeat", sessions);
    expect(result.current.length).toBeGreaterThanOrEqual(1);
    expect(result.current[0].session.branch).toBe("my-feature-branch");
  });

  // T-UNIT-TS-005
  it("excludes UNSPECIFIED sessions from results", () => {
    const sessions = [
      makeSession({
        id: "active",
        title: "auth-service",
        status: SessionStatus.RUNNING,
      }),
      makeSession({
        id: "unspecified",
        title: "auth-unspecified",
        status: SessionStatus.UNSPECIFIED,
      }),
    ];
    const { result } = renderWithStore("auth", sessions);
    const ids = result.current.map((r) => r.session.id);
    expect(ids).toContain("active");
    expect(ids).not.toContain("unspecified");
  });

  it("includes PAUSED sessions in results", () => {
    const sessions = [
      makeSession({
        id: "paused",
        title: "auth-service",
        status: SessionStatus.PAUSED,
      }),
    ];
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBe(1);
    expect(result.current[0].session.id).toBe("paused");
  });

  it("includes NEEDS_APPROVAL sessions in results", () => {
    const sessions = [
      makeSession({
        id: "approval",
        title: "auth-service",
        status: SessionStatus.NEEDS_APPROVAL,
      }),
    ];
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBe(1);
    expect(result.current[0].session.id).toBe("approval");
  });

  // T-UNIT-TS-006
  it("caps results at 8 even when more sessions match", () => {
    const sessions = Array.from({ length: 20 }, (_, i) =>
      makeSession({ id: `session-${i}`, title: `auth-service-${i}` })
    );
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBeLessThanOrEqual(8);
  });

  // T-UNIT-TS-007
  it("session matching both title and branch scores higher than session matching only title", () => {
    const sessions = [
      makeSession({
        id: "both-fields",
        title: "auth-service",
        branch: "auth-feature",
      }),
      makeSession({
        id: "title-only",
        title: "auth-handler",
        branch: "main",
      }),
    ];
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBeGreaterThanOrEqual(2);
    expect(result.current[0].session.id).toBe("both-fields");
  });

  it("score property is between 0 and 1 (Fuse.js convention)", () => {
    const sessions = [makeSession({ title: "auth-service" })];
    const { result } = renderWithStore("auth", sessions);
    expect(result.current.length).toBeGreaterThan(0);
    const score = result.current[0].score;
    expect(score).toBeGreaterThanOrEqual(0);
    expect(score).toBeLessThanOrEqual(1);
  });

  it("returns empty when no sessions match", () => {
    const sessions = [makeSession({ title: "completely-unrelated" })];
    const { result } = renderWithStore("zzzyyyxxx", sessions);
    expect(result.current).toEqual([]);
  });

  it("returns empty when store has no sessions", () => {
    const { result } = renderWithStore("auth", []);
    expect(result.current).toEqual([]);
  });
});
