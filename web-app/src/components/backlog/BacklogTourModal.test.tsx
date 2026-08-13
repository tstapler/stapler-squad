/**
 * Tests for BacklogTourModal.
 *
 * Covers:
 *  1. Does not render step content when isOpen is false
 *  2. Renders the first step (lifecycle) when open
 *  3. Next/Back navigate between all 4 steps
 *  4. Step 2 explicitly explains the Repository Path gotcha
 *  5. Skip calls onComplete(true)
 *  6. "Got it" with the default (checked) checkbox calls onComplete(true)
 *  7. Unchecking "Don't show this again" then "Got it" calls onComplete(false)
 *     — regression test for the checkbox being a no-op (stapler-squad#152 review)
 *  8. Dismissing via the dialog's onOpenChange (backdrop/Escape) respects the
 *     current checkbox state, same as clicking "Got it"
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { BacklogTourModal } from "./BacklogTourModal";

describe("BacklogTourModal — visibility", () => {
  it("renders nothing when isOpen is false", () => {
    render(<BacklogTourModal isOpen={false} onComplete={jest.fn()} />);
    expect(screen.queryByTestId("backlog-tour-modal")).not.toBeInTheDocument();
  });

  it("renders the first step when isOpen is true", () => {
    render(<BacklogTourModal isOpen onComplete={jest.fn()} />);
    expect(screen.getByText("How backlog items work")).toBeInTheDocument();
  });
});

describe("BacklogTourModal — navigation", () => {
  it("Next advances through all steps, Back returns to the previous one", () => {
    render(<BacklogTourModal isOpen onComplete={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Filling out the form")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("What happens after you hit Create")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Skip planning / Skip review gate")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByText("What happens after you hit Create")).toBeInTheDocument();
  });

  it("step 2 calls out the Repository Path gotcha explicitly", () => {
    render(<BacklogTourModal isOpen onComplete={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByTestId("backlog-tour-repo-path-callout")).toHaveTextContent(
      "we'll clone it for you automatically"
    );
  });
});

function goToLastStep() {
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
}

describe("BacklogTourModal — dismissal", () => {
  it("Skip calls onComplete(true)", () => {
    const onComplete = jest.fn();
    render(<BacklogTourModal isOpen onComplete={onComplete} />);

    fireEvent.click(screen.getByRole("button", { name: "Skip tour" }));

    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(onComplete).toHaveBeenCalledWith(true);
  });

  it("'Got it' with the default checked checkbox calls onComplete(true)", () => {
    const onComplete = jest.fn();
    render(<BacklogTourModal isOpen onComplete={onComplete} />);

    goToLastStep();
    fireEvent.click(screen.getByRole("button", { name: "Got it" }));

    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(onComplete).toHaveBeenCalledWith(true);
  });

  it("unchecking 'Don't show this again' then 'Got it' calls onComplete(false)", () => {
    const onComplete = jest.fn();
    render(<BacklogTourModal isOpen onComplete={onComplete} />);

    goToLastStep();
    fireEvent.click(screen.getByRole("checkbox", { name: "Don't show this again" }));
    fireEvent.click(screen.getByRole("button", { name: "Got it" }));

    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(onComplete).toHaveBeenCalledWith(false);
  });

  it("dismissing via onOpenChange respects the current checkbox state", () => {
    const onComplete = jest.fn();
    const { unmount } = render(<BacklogTourModal isOpen onComplete={onComplete} />);

    goToLastStep();
    fireEvent.click(screen.getByRole("checkbox", { name: "Don't show this again" }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(onComplete).toHaveBeenCalledWith(false);
    unmount();
  });
});
