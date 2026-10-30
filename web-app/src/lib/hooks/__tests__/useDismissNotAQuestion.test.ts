import { renderHook } from "@testing-library/react";
import { resolveNotAQuestionDismissKind, useDismissNotAQuestion } from "../useDismissNotAQuestion";

describe("useDismissNotAQuestion", () => {
  // AcknowledgeSessionRequest has no dismiss_kind in the current generated client, so the action
  // must stay unavailable rather than falling back to a plain acknowledge.
  it("is unavailable while the generated request lacks dismiss_kind", () => {
    expect(resolveNotAQuestionDismissKind()).toBeNull();
    const { result } = renderHook(() => useDismissNotAQuestion());
    expect(result.current).toBeUndefined();
  });
});
