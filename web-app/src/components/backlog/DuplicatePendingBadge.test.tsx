import React from "react";
import { render, screen } from "@testing-library/react";
import { DuplicatePendingBadge } from "./DuplicatePendingBadge";

describe("DuplicatePendingBadge", () => {
  it("renders a distinct awaiting-confirmation badge naming the claimed duplicate", () => {
    render(<DuplicatePendingBadge duplicateRef="https://github.com/o/r/pull/801" />);
    const badge = screen.getByTestId("duplicate-pending-badge");
    expect(badge).toHaveTextContent("Duplicate? awaiting confirmation");
    expect(badge).toHaveAttribute("title", expect.stringContaining("pull/801"));
  });

  it("falls back to generic title when no ref is known", () => {
    render(<DuplicatePendingBadge duplicateRef="" />);
    expect(screen.getByTestId("duplicate-pending-badge")).toHaveAttribute(
      "title",
      "Duplicate claimed — awaiting confirmation"
    );
  });
});
