import React from "react";
import { render, screen } from "@testing-library/react";
import { BoardCard } from "./BoardCard";
import type { ColumnKey } from "./session-columns";
import type { Session } from "@/gen/session/v1/types_pb";

// Story 2.2.3: BoardCard is a thin wrapper that spreads its SessionCardProps straight
// through to SessionCard (see BoardCard.tsx:35,68) -- stub SessionCard so this test only
// exercises that forwarding, not SessionCard's own rendering (mirrors SessionBoard.test.tsx's
// mocking strategy for the same component).
jest.mock("./SessionCard", () => ({
  SessionCard: ({ visibleColumns }: { visibleColumns?: ColumnKey[] }) => (
    <div data-testid="session-card-stub">{visibleColumns ? visibleColumns.join(",") : "default"}</div>
  ),
}));

const minimalSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

describe("BoardCard — visibleColumns forwarding (Story 2.2.3)", () => {
  it("BoardCard_should_ForwardVisibleColumnsToSessionCard_When_PropProvided", () => {
    const session = { ...minimalSession } as unknown as Session;
    render(
      <BoardCard
        rowKey="__default__"
        session={session}
        visibleColumns={["agent", "memory"]}
      />
    );

    expect(screen.getByTestId("session-card-stub")).toHaveTextContent("agent,memory");
  });

  it("BoardCard_should_LetSessionCardApplyItsOwnDefault_When_VisibleColumnsOmitted", () => {
    const session = { ...minimalSession } as unknown as Session;
    render(<BoardCard rowKey="__default__" session={session} />);

    expect(screen.getByTestId("session-card-stub")).toHaveTextContent("default");
  });
});
