/**
 * Tests for useSessionRepoPaths hook.
 *
 * Covers R3 (validation.md): the hook propagates the recency order produced by
 * selectActiveSessionsSortedByUpdatedAt, deduplicated, dropping UNSPECIFIED sessions.
 */

import React from "react";
import { renderHook } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import sessionsReducer, { setSessions } from "@/lib/store/sessionsSlice";
import { useSessionRepoPaths } from "./useSessionRepoPaths";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStore(sessions: Session[] = []) {
  const store = configureStore({
    reducer: {
      sessions: sessionsReducer,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
  if (sessions.length > 0) {
    store.dispatch(setSessions(sessions));
  }
  return store;
}

function renderWithStore(sessions: Session[]) {
  const store = makeStore(sessions);
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return renderHook(() => useSessionRepoPaths(), { wrapper: Wrapper });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useSessionRepoPaths — recency-ordered paths", () => {
  it("returns deduplicated paths in selectActiveSessionsSortedByUpdatedAt order, excluding UNSPECIFIED sessions", () => {
    const s1 = create(SessionSchema, {
      id: "s1",
      path: "/repo/a",
      status: SessionStatus.ACTIVE,
      updatedAt: { seconds: 300n, nanos: 0 },
    });
    const s2 = create(SessionSchema, {
      id: "s2",
      path: "/repo/b",
      status: SessionStatus.ACTIVE,
      updatedAt: { seconds: 100n, nanos: 0 },
    });
    const s3 = create(SessionSchema, {
      id: "s3",
      path: "/repo/c",
      status: SessionStatus.UNSPECIFIED,
    });

    const { result } = renderWithStore([s1, s2, s3]);

    expect(result.current).toEqual(["/repo/a", "/repo/b"]);
  });
});
