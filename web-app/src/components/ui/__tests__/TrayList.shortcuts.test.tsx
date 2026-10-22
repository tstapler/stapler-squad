import { fireEvent, render, screen } from "@testing-library/react";
import type { TrayRow } from "@/lib/utils/notificationGrouping";
import { TrayList, type ListRow } from "../TrayList";

jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: () => ({
    getVirtualItems: () => [0, 1, 2].map((index) => ({ index, start: index * 44, key: index })),
    getTotalSize: () => 132,
    measureElement: () => undefined,
    scrollToIndex: () => undefined,
  }),
}));

const rows: ListRow[] = [0, 1, 2].map((i): TrayRow => ({ kind: "note", key: `note-${i}`, text: `note ${i}` }) as TrayRow);

const noop = () => undefined;
function renderList() {
  return render(
    <TrayList
      rows={rows}
      setSize={3}
      itemProps={{} as never}
      scrollRef={{ current: null }}
      hasMore={false}
      loading={false}
      onLoadMore={noop}
      onToggleGroup={noop}
      onDismissGroup={noop}
      onDismissSession={noop}
      groupForId={() => undefined}
    />,
  );
}

const tabStop = () => screen.getAllByRole("listitem").find((el) => el.getAttribute("tabindex") === "0")?.dataset.trayRow;

describe("TrayList item shortcuts (ux.md section 7)", () => {
  it("moves item focus the same way for ArrowDown and j", () => {
    renderList();
    const first = screen.getAllByRole("listitem")[0];
    fireEvent.keyDown(first, { key: "ArrowDown" });
    expect(tabStop()).toBe("note-1");
    fireEvent.keyDown(screen.getAllByRole("listitem")[1], { key: "k" });
    expect(tabStop()).toBe("note-0");
    fireEvent.keyDown(screen.getAllByRole("listitem")[0], { key: "j" });
    expect(tabStop()).toBe("note-1");
  });

  it.each([
    ["a textarea", "textarea", ""],
    ["an input", "input", ""],
    ["the terminal container", "span", "xterm"],
  ])("ignores j and arrows typed into %s", (_name, tag, className) => {
    renderList();
    const target = document.createElement(tag);
    target.className = className;
    screen.getAllByRole("listitem")[0].appendChild(target);
    fireEvent.keyDown(target, { key: "j" });
    fireEvent.keyDown(target, { key: "ArrowDown" });
    expect(tabStop()).toBe("note-0");
  });

  it("ignores j while Ctrl is held and during IME composition", () => {
    renderList();
    const first = screen.getAllByRole("listitem")[0];
    fireEvent.keyDown(first, { key: "j", ctrlKey: true });
    expect(tabStop()).toBe("note-0");
    fireEvent.keyDown(first, { key: "j", isComposing: true });
    expect(tabStop()).toBe("note-0");
  });
});
