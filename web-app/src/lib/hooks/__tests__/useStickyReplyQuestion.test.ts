import { renderHook } from "@testing-library/react";
import { useStickyReplyQuestion } from "../useStickyReplyQuestion";

type Q = { id: string };

describe("useStickyReplyQuestion", () => {
  it("keeps_the_question_after_it_turns_read_so_the_receipt_stays", () => {
    const q: Q = { id: "n1" };
    const { result, rerender } = renderHook(({ unread }: { unread: Q | undefined }) => useStickyReplyQuestion(unread, "s1"), {
      initialProps: { unread: q as Q | undefined },
    });
    expect(result.current).toBe(q);
    rerender({ unread: undefined });
    expect(result.current).toBe(q);
  });

  it("is_replaced_by_a_different_unread_question", () => {
    const a: Q = { id: "a" };
    const b: Q = { id: "b" };
    const { result, rerender } = renderHook(({ unread }: { unread: Q | undefined }) => useStickyReplyQuestion(unread, "s1"), {
      initialProps: { unread: a as Q | undefined },
    });
    rerender({ unread: b });
    expect(result.current).toBe(b);
  });

  it("does_not_carry_a_question_across_sessions", () => {
    const a: Q = { id: "a" };
    const { result, rerender } = renderHook(
      ({ unread, sid }: { unread: Q | undefined; sid: string }) => useStickyReplyQuestion(unread, sid),
      { initialProps: { unread: a as Q | undefined, sid: "s1" } },
    );
    rerender({ unread: undefined, sid: "s2" });
    expect(result.current).toBeUndefined();
  });

  it("starts_empty_when_nothing_is_pending", () => {
    const { result } = renderHook(() => useStickyReplyQuestion<Q>(undefined, "s1"));
    expect(result.current).toBeUndefined();
  });
});
