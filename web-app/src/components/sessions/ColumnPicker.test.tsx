import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ColumnPicker } from "./ColumnPicker";
import { DEFAULT_VISIBLE_COLUMNS, ColumnKey } from "./session-columns";

describe("ColumnPicker", () => {
  function renderPicker(visibleColumns: ColumnKey[] = DEFAULT_VISIBLE_COLUMNS) {
    const onChange = jest.fn();
    render(
      <ColumnPicker
        visibleColumns={visibleColumns}
        onChange={onChange}
        open={true}
        onOpenChange={jest.fn()}
      />,
    );
    return { onChange };
  }

  it("ColumnPicker_should_ReintroduceAgentColumn_When_UserChecksAgentToggle", () => {
    const { onChange } = renderPicker(DEFAULT_VISIBLE_COLUMNS);

    const agentCheckbox = screen.getByLabelText("Agent") as HTMLInputElement;
    expect(agentCheckbox.checked).toBe(false);

    fireEvent.click(agentCheckbox);

    expect(onChange).toHaveBeenCalledTimes(1);
    const nextVisibleColumns = onChange.mock.calls[0][0] as ColumnKey[];
    expect(nextVisibleColumns).toContain("agent");
  });

  it("ColumnPicker_should_ListAgentAndMemoryAsUncheckedByDefault_When_UsingDefaultVisibleColumns", () => {
    renderPicker(DEFAULT_VISIBLE_COLUMNS);

    expect((screen.getByLabelText("Agent") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByLabelText("Memory") as HTMLInputElement).checked).toBe(false);
  });
});
