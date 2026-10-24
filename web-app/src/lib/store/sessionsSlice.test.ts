/**
 * Characterization tests (Story 1.4.1b): other consumers of Session.updatedAt must keep
 * their current semantics when the attention idle clock moves to lastMeaningfulOutput.
 */
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";
import reducer, {
  selectActiveSessionsSortedByUpdatedAt,
  setSessions,
  upsertSession,
} from "./sessionsSlice";

function session(id: string, updatedSeconds: number, opts: { createdSeconds?: number; title?: string; status?: SessionStatus } = {}) {
  return create(SessionSchema, {
    id,
    title: opts.title ?? id,
    status: opts.status ?? SessionStatus.ACTIVE,
    updatedAt: { seconds: BigInt(updatedSeconds), nanos: 0 },
    createdAt: { seconds: BigInt(opts.createdSeconds ?? 0), nanos: 0 },
  });
}

const rootOf = (sessions: ReturnType<typeof reducer>) => ({ sessions }) as unknown as RootState;

describe("sessionsSlice updatedAt consumers", () => {
  describe("selectActiveSessionsSortedByUpdatedAt", () => {
    it("orders by updatedAt descending, then createdAt descending, then id", () => {
      const state = reducer(
        undefined,
        setSessions([
          session("b", 100, { createdSeconds: 1 }),
          session("a", 100, { createdSeconds: 1 }),
          session("newer-created", 100, { createdSeconds: 5 }),
          session("latest", 200),
          session("oldest", 50),
        ])
      );
      const ids = selectActiveSessionsSortedByUpdatedAt(rootOf(state)).map((s) => s.id);
      expect(ids).toEqual(["latest", "newer-created", "a", "b", "oldest"]);
    });

    it("excludes sessions with an UNSPECIFIED status", () => {
      const state = reducer(
        undefined,
        setSessions([session("kept", 10), session("dropped", 20, { status: SessionStatus.UNSPECIFIED })])
      );
      expect(selectActiveSessionsSortedByUpdatedAt(rootOf(state)).map((s) => s.id)).toEqual(["kept"]);
    });
  });

  describe("reducer dedup on equal updatedAt", () => {
    it("setSessions keeps the existing entity reference when updatedAt is unchanged", () => {
      const first = reducer(undefined, setSessions([session("s", 100, { title: "old" })]));
      const second = reducer(first, setSessions([session("s", 100, { title: "renamed" })]));
      expect(second.entities["s"]).toBe(first.entities["s"]);
      expect(second.entities["s"]?.title).toBe("old");
    });

    it("setSessions replaces the entity when updatedAt changes", () => {
      const first = reducer(undefined, setSessions([session("s", 100, { title: "old" })]));
      const second = reducer(first, setSessions([session("s", 101, { title: "renamed" })]));
      expect(second.entities["s"]?.title).toBe("renamed");
    });

    it("upsertSession skips a no-op upsert with equal updatedAt", () => {
      const first = reducer(undefined, upsertSession(session("s", 100, { title: "old" })));
      const second = reducer(first, upsertSession(session("s", 100, { title: "renamed" })));
      expect(second.entities["s"]).toBe(first.entities["s"]);
    });

    it("upsertSession applies an upsert when updatedAt differs", () => {
      const first = reducer(undefined, upsertSession(session("s", 100, { title: "old" })));
      const second = reducer(first, upsertSession(session("s", 101, { title: "renamed" })));
      expect(second.entities["s"]?.title).toBe("renamed");
    });
  });
});
