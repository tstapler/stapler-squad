import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { DependencyChip } from "./DependencyChip";

const dep = (title: string, status: string) => ({ itemId: `id-${title}`, publicId: "", title, status });

describe("DependencyChip", () => {
  it("renders nothing for empty or all-resolved blockers", () => {
    const { container, rerender } = render(<DependencyChip deps={[]} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<DependencyChip deps={[dep("A", "done"), dep("B", "archived")]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the first unresolved blocker and lists the rest in the +N more title", () => {
    render(<DependencyChip deps={[dep("Gone", "done"), dep("A", "ready"), dep("B", "in_progress"), dep("C", "ready")]} />);
    expect(screen.getByTestId("dependency-chip")).toHaveTextContent("A");
    const more = screen.getByTestId("dependency-chip-more");
    expect(more).toHaveTextContent("+2 more");
    expect(more).toHaveAttribute("title", "B (in_progress)\nC (ready)");
  });

  it("activates via click, Enter and Space, and is focusable", () => {
    const onItemClick = jest.fn();
    render(<DependencyChip deps={[dep("A", "ready")]} onItemClick={onItemClick} />);
    const chip = screen.getByRole("button");
    expect(chip).toHaveAttribute("tabindex", "0");
    fireEvent.click(chip);
    fireEvent.keyDown(chip, { key: "Enter" });
    fireEvent.keyDown(chip, { key: " " });
    fireEvent.keyDown(chip, { key: "a" });
    expect(onItemClick).toHaveBeenCalledTimes(3);
    expect(onItemClick).toHaveBeenCalledWith("id-A");
  });

  it("does not crash without onItemClick", () => {
    render(<DependencyChip deps={[dep("A", "ready")]} />);
    fireEvent.click(screen.getByTestId("dependency-chip"));
  });

  it("does not bubble clicks to a parent", () => {
    const parent = jest.fn();
    render(
      <div onClick={parent}>
        <DependencyChip deps={[dep("A", "ready")]} onItemClick={jest.fn()} />
      </div>
    );
    fireEvent.click(screen.getByTestId("dependency-chip"));
    expect(parent).not.toHaveBeenCalled();
  });
});
