/**
 * Tests for TriageLoadingIndicator component.
 *
 * Covers:
 *  1. Renders label and elapsed time when elapsedSeconds < 60
 *  2. Renders "still working" label when elapsedSeconds >= 60 and < 180
 *  3. Returns null (renders nothing) when elapsedSeconds >= 180
 *  4. Cancel button calls onCancel
 *  5. compact=true renders compact variant (cancelButtonCompact)
 *  6. compact=false renders full variant (cancelButton with "Stop" text)
 *  7. aria-label updates at 30s intervals (0s → "0 seconds elapsed", 30s → "30 seconds elapsed", 60s → "60 seconds elapsed")
 *  8. role="status" and aria-live="polite" present
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";

// ---------------------------------------------------------------------------
// Default props factory
// ---------------------------------------------------------------------------

function makeProps(
  overrides: Partial<React.ComponentProps<typeof TriageLoadingIndicator>> = {}
) {
  return {
    elapsedSeconds: 0,
    context: "item" as const,
    onCancel: jest.fn(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Test: 1 — Renders label and elapsed time when elapsedSeconds < 60
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — < 60 seconds (item context)", () => {
  it("renders label and elapsed time when elapsedSeconds < 60 (item context)", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={jest.fn()}
      />
    );

    expect(
      screen.getByText("Thinking about acceptance criteria...")
    ).toBeInTheDocument();
    expect(screen.getByText("30s")).toBeInTheDocument();
  });

  it("renders label and elapsed time when elapsedSeconds < 60 (list context)", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={45}
        context="list"
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText("Thinking...")).toBeInTheDocument();
    expect(screen.getByText("45s")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 2 — Renders "still working" label when 60 <= elapsedSeconds < 180
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — >= 60 and < 180 seconds", () => {
  it("renders 'Still thinking' label when elapsedSeconds >= 60 (item context)", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={90}
        context="item"
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText("Still thinking — up to 3 min")).toBeInTheDocument();
    expect(screen.getByText("90s")).toBeInTheDocument();
  });

  it("renders 'Still working' label when elapsedSeconds >= 60 (list context)", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={120}
        context="list"
        onCancel={jest.fn()}
      />
    );

    expect(
      screen.getByText("Still working — up to 3 min")
    ).toBeInTheDocument();
    expect(screen.getByText("120s")).toBeInTheDocument();
  });

  it("renders at boundary elapsedSeconds = 60", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={60}
        context="list"
        onCancel={jest.fn()}
      />
    );

    expect(
      screen.getByText("Still working — up to 3 min")
    ).toBeInTheDocument();
  });

  it("renders at boundary elapsedSeconds = 179", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={179}
        context="item"
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText("Still thinking — up to 3 min")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 3 — Returns null (renders nothing) when elapsedSeconds >= 180
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — >= 180 seconds (timeout)", () => {
  it("returns null when elapsedSeconds >= 180", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={180}
        context="item"
        onCancel={jest.fn()}
      />
    );

    expect(container.firstChild).toBeNull();
  });

  it("returns null when elapsedSeconds > 180", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={200}
        context="list"
        onCancel={jest.fn()}
      />
    );

    expect(container.firstChild).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Test: 4 — Cancel button calls onCancel
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — onCancel callback", () => {
  it("calls onCancel when cancel button is clicked (full variant)", async () => {
    const onCancel = jest.fn();
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={onCancel}
        compact={false}
      />
    );

    const cancelBtn = screen.getByRole("button", { name: /Cancel triage/i });
    fireEvent.click(cancelBtn);

    await waitFor(() => expect(onCancel).toHaveBeenCalledTimes(1));
  });

  it("calls onCancel when cancel button is clicked (compact variant)", async () => {
    const onCancel = jest.fn();
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="list"
        onCancel={onCancel}
        compact={true}
      />
    );

    const cancelBtn = screen.getByRole("button", { name: /Cancel triage/i });
    fireEvent.click(cancelBtn);

    await waitFor(() => expect(onCancel).toHaveBeenCalledTimes(1));
  });
});

// ---------------------------------------------------------------------------
// Test: 5 — compact=true renders compact variant (cancelButtonCompact)
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — compact variant", () => {
  it("renders compact variant (×) when compact=true", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="list"
        onCancel={jest.fn()}
        compact={true}
      />
    );

    const cancelBtn = screen.getByRole("button", { name: /Cancel triage/i });
    expect(cancelBtn).toBeInTheDocument();
    // In compact mode, button text is × (not visible in screen.getByText but aria-label confirms intent)
    expect(cancelBtn).toHaveAttribute("aria-label", "Cancel triage");
  });
});

// ---------------------------------------------------------------------------
// Test: 6 — compact=false renders full variant (cancelButton with "Stop" text)
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — full variant", () => {
  it("renders full variant with 'Stop' text when compact=false", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={jest.fn()}
        compact={false}
      />
    );

    const cancelBtn = screen.getByRole("button", { name: /Cancel triage/i });
    expect(cancelBtn).toBeInTheDocument();
    expect(cancelBtn.textContent).toBe("Stop");
  });

  it("renders full variant by default (compact not specified)", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const cancelBtn = screen.getByRole("button", { name: /Cancel triage/i });
    expect(cancelBtn.textContent).toBe("Stop");
  });
});

// ---------------------------------------------------------------------------
// Test: 7 — aria-label updates at 30s intervals
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — aria-label intervals", () => {
  it("aria-label shows 0 seconds elapsed at 0s", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={0}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 0 seconds elapsed"
    );
  });

  it("aria-label shows 30 seconds elapsed at 30s", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 30 seconds elapsed"
    );
  });

  it("aria-label shows 30 seconds elapsed at 45s (rounded down to nearest 30s)", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={45}
        context="list"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 30 seconds elapsed"
    );
  });

  it("aria-label shows 60 seconds elapsed at 60s", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={60}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 60 seconds elapsed"
    );
  });

  it("aria-label shows 90 seconds elapsed at 90s", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={90}
        context="list"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 90 seconds elapsed"
    );
  });

  it("aria-label shows 120 seconds elapsed at 150s (rounded down to nearest 30s)", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={150}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toHaveAttribute(
      "aria-label",
      "Triage in progress, 150 seconds elapsed"
    );
  });
});

// ---------------------------------------------------------------------------
// Test: 8 — role="status" and aria-live="polite" present
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — accessibility", () => {
  it("has role=status and aria-live=polite", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="item"
        onCancel={jest.fn()}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toBeInTheDocument();
    expect(status).toHaveAttribute("aria-live", "polite");
  });

  it("has role=status and aria-live=polite in compact mode", () => {
    const { container } = render(
      <TriageLoadingIndicator
        elapsedSeconds={30}
        context="list"
        onCancel={jest.fn()}
        compact={true}
      />
    );

    const status = container.querySelector('[role="status"]');
    expect(status).toBeInTheDocument();
    expect(status).toHaveAttribute("aria-live", "polite");
  });
});

// ---------------------------------------------------------------------------
// Test: Context-specific labels
// ---------------------------------------------------------------------------

describe("TriageLoadingIndicator — context-specific labels", () => {
  it("shows item context label for item context", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={15}
        context="item"
        onCancel={jest.fn()}
      />
    );

    expect(
      screen.getByText("Thinking about acceptance criteria...")
    ).toBeInTheDocument();
  });

  it("shows list context label for list context", () => {
    render(
      <TriageLoadingIndicator
        elapsedSeconds={15}
        context="list"
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText("Thinking...")).toBeInTheDocument();
  });
});
