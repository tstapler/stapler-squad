import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { ActionKind } from "@/gen/session/v1/notification_action_pb";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import { INLINE_ACTION_UNDO_EVENT, useInlineActionTelemetry } from "../useInlineActionTelemetry";

describe("useInlineActionTelemetry", () => {
  it("tracks an undo with the lower-cased action kind", () => {
    const track = jest.fn();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <AnalyticsContext.Provider value={{ provider: { metadata: { name: "t" }, track }, track }}>{children}</AnalyticsContext.Provider>
    );
    const { result } = renderHook(() => useInlineActionTelemetry(), { wrapper });
    result.current.trackUndo(ActionKind.BACKLOG_DECISION);
    expect(track).toHaveBeenCalledWith(
      expect.objectContaining({ name: INLINE_ACTION_UNDO_EVENT, category: "user_action", labels: { kind: "backlog_decision" } }),
    );
  });

  it("is inert without an analytics provider", () => {
    const { result } = renderHook(() => useInlineActionTelemetry());
    expect(() => result.current.trackUndo(ActionKind.APPROVE)).not.toThrow();
  });
});
