import { render, screen, fireEvent } from "@testing-library/react";
import { LogRow } from "./LogRow";
import type { LogEntry } from "@/lib/hooks/useLogViewer";

const entry = (level: LogEntry["level"]): LogEntry =>
  ({ id: "1", timestamp: "2026-01-02T15:04:05.000Z", message: "hello", level }) as LogEntry;

const base = { index: 0, isExpanded: false, isSelected: false, searchQuery: "" };

describe("LogRow level badge", () => {
  it("badge_should_FilterByLevel_WithoutTogglingRow_WhenCallbackProvided", () => {
    const onToggle = jest.fn();
    const onFilterByLevel = jest.fn();
    render(<LogRow {...base} entry={entry("ERROR")} onToggle={onToggle} onFilterByLevel={onFilterByLevel} />);

    fireEvent.click(screen.getByRole("button", { name: "Filter by ERROR" }));

    expect(screen.getByRole("button", { name: "Filter by ERROR" })).toHaveAttribute("tabindex", "-1");
    expect(onFilterByLevel).toHaveBeenCalledWith("ERROR");
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("badge_should_NotBeAButton_WhenCallbackAbsent_AndClickStillTogglesRow", () => {
    const onToggle = jest.fn();
    render(<LogRow {...base} entry={entry("WARN")} onToggle={onToggle} />);

    expect(screen.queryByRole("button")).toBeNull();
    fireEvent.click(screen.getByText("WARN"));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("badge_should_StayPlainText_ForUnknownLevel", () => {
    render(<LogRow {...base} entry={entry("UNKNOWN")} onToggle={jest.fn()} onFilterByLevel={jest.fn()} />);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("badge_keyboard_should_NotExpandRow", () => {
    const onToggle = jest.fn();
    render(<LogRow {...base} entry={entry("INFO")} onToggle={onToggle} onFilterByLevel={jest.fn()} />);
    fireEvent.keyDown(screen.getByRole("button", { name: "Filter by INFO" }), { key: "Enter" });
    expect(onToggle).not.toHaveBeenCalled();
  });
});
