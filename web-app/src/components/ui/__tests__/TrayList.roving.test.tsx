import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import type { TrayRow } from "@/lib/utils/notificationGrouping";
import { TrayList, type ListRow } from "../TrayList";

let mockWindow: number[] = [0, 1, 2];

jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: () => ({
    getVirtualItems: () => mockWindow.map((index) => ({ index, start: index * 44, key: index })),
    getTotalSize: () => 44 * 20,
    measureElement: () => undefined,
    scrollToIndex: () => undefined,
  }),
}));

const rows: ListRow[] = Array.from({ length: 20 }, (_, i): TrayRow => ({
  kind: "note",
  key: `note-${i}`,
  text: `note ${i}`,
}) as TrayRow);

function tree() {
  return (
    <TrayList
      rows={rows}
      setSize={rows.length}
      itemProps={{} as never}
      scrollRef={{ current: null }}
      hasMore={false}
      loading={false}
      onLoadMore={() => undefined}
      onToggleGroup={() => undefined}
      onDismissGroup={() => undefined}
      onDismissSession={() => undefined}
      groupForId={() => undefined}
    />
  );
}

describe("TrayList roving tabindex under virtualization", () => {
  it("keeps exactly one tab stop on a mounted row when the active row scrolls out of the window", () => {
    mockWindow = [0, 1, 2];
    const view = render(tree());
    fireEvent.focus(screen.getAllByRole("listitem")[1]);
    expect(screen.getAllByRole("listitem")[1]).toHaveAttribute("tabindex", "0");

    mockWindow = [10, 11, 12];
    view.rerender(tree());
    const stops = screen.getAllByRole("listitem").filter((el) => el.getAttribute("tabindex") === "0");
    expect(stops).toHaveLength(1);
    expect(stops[0]).toHaveAttribute("data-tray-row", "note-10");
  });
});
