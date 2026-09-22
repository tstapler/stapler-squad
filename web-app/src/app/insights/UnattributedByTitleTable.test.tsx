import React from "react";
import { render, screen } from "@testing-library/react";
import { UnattributedByTitleTable } from "./UnattributedByTitleTable";
import type { ItemRoleCost } from "@/gen/session/v1/insights_pb";

function makeItem(overrides: Partial<ItemRoleCost> = {}): ItemRoleCost {
  return {
    $typeName: "session.v1.ItemRoleCost",
    itemId: "untracked:kibitzer",
    itemTitle: "kibitzer",
    estimatedCostUsd: 203.9,
    sessionCount: 1,
    unpricedSessionCount: 0,
    ...overrides,
  } as ItemRoleCost;
}

describe("UnattributedByTitleTable", () => {
  it("renders one row per session title", () => {
    render(
      <UnattributedByTitleTable
        items={[
          makeItem({ itemId: "untracked:kibitzer", itemTitle: "kibitzer", estimatedCostUsd: 203.9 }),
          makeItem({ itemId: "untracked:steam-controls", itemTitle: "steam-controls", estimatedCostUsd: 124.1, sessionCount: 2 }),
        ]}
      />,
    );

    expect(screen.getByText("kibitzer")).toBeInTheDocument();
    expect(screen.getByText("$203.90")).toBeInTheDocument();
    expect(screen.getByText("steam-controls")).toBeInTheDocument();
    expect(screen.getByText("$124.10")).toBeInTheDocument();
  });

  it("renders nothing when items is empty", () => {
    const { container } = render(<UnattributedByTitleTable items={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
