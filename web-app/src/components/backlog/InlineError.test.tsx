/**
 * Tests for InlineError component.
 *
 * Covers:
 *  1. Renders "transient" type with retry and dismiss buttons
 *  2. Renders "timeout" type with appropriate message
 *  3. Renders "permanent" type with link (if present)
 *  4. onRetry callback fires on retry button click
 *  5. onDismiss callback fires on dismiss button click
 *  6. role="alert" and aria-live="assertive" are present
 *  7. External links have rel="noopener noreferrer"
 *  8. Does NOT render unknown type (edge case if applicable)
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { InlineError } from "./InlineError";

// ---------------------------------------------------------------------------
// Default props factory
// ---------------------------------------------------------------------------

function makeProps(overrides: Partial<React.ComponentProps<typeof InlineError>> = {}) {
  return {
    type: "transient" as const,
    onRetry: jest.fn(),
    onDismiss: jest.fn(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Test: 1 — transient type with retry and dismiss buttons
// ---------------------------------------------------------------------------

describe("InlineError — transient type", () => {
  it("renders transient type with retry and dismiss buttons", () => {
    const onRetry = jest.fn();
    const onDismiss = jest.fn();
    const { container } = render(
      <InlineError
        type="transient"
        onRetry={onRetry}
        onDismiss={onDismiss}
      />
    );

    expect(screen.getByText("Triage failed")).toBeInTheDocument();
    expect(
      container.textContent?.includes("Network error. The request could not be completed.")
    ).toBe(true);

    const retryBtn = screen.getByRole("button", { name: /Retry/i });
    const dismissBtn = screen.getByRole("button", { name: /Dismiss error/i });

    expect(retryBtn).toBeInTheDocument();
    expect(dismissBtn).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 2 — timeout type with appropriate message
// ---------------------------------------------------------------------------

describe("InlineError — timeout type", () => {
  it("renders timeout type with appropriate message", () => {
    const { container } = render(
      <InlineError
        type="timeout"
        onRetry={jest.fn()}
      />
    );

    expect(screen.getByText("Triage timed out")).toBeInTheDocument();
    expect(
      container.textContent?.includes("The triage session did not complete within 3 minutes.")
    ).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Test: 3 — permanent type with link (if present)
// ---------------------------------------------------------------------------

describe("InlineError — permanent type", () => {
  it("renders permanent type with session logs link when logsSessionId is provided", () => {
    const logsSessionId = "test-session-123";
    render(
      <InlineError
        type="permanent"
        onRetry={jest.fn()}
        logsSessionId={logsSessionId}
      />
    );

    expect(screen.getByText("Triage failed")).toBeInTheDocument();
    expect(
      screen.getByText(
        "The triage session exited unexpectedly (exit code 1). Check the session logs for details."
      )
    ).toBeInTheDocument();

    const logsLink = screen.getByRole("link", { name: /View session logs/i });
    expect(logsLink).toBeInTheDocument();
    expect(logsLink).toHaveAttribute("href", "/sessions/test-session-123/logs");
  });

  it("renders permanent type without link when logsSessionId is not provided", () => {
    render(
      <InlineError
        type="permanent"
        onRetry={jest.fn()}
      />
    );

    expect(screen.getByText("Triage failed")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /View session logs/i })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 4 — onRetry callback fires on retry button click
// ---------------------------------------------------------------------------

describe("InlineError — onRetry callback", () => {
  it("fires onRetry callback when retry button is clicked on transient error", async () => {
    const onRetry = jest.fn();
    render(
      <InlineError
        type="transient"
        onRetry={onRetry}
      />
    );

    const retryBtn = screen.getByRole("button", { name: /Retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => expect(onRetry).toHaveBeenCalledTimes(1));
  });

  it("fires onRetry callback when retry button is clicked on timeout error", async () => {
    const onRetry = jest.fn();
    render(
      <InlineError
        type="timeout"
        onRetry={onRetry}
      />
    );

    const retryBtn = screen.getByRole("button", { name: /Retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => expect(onRetry).toHaveBeenCalledTimes(1));
  });

  it("fires onRetry callback when retry button is clicked on permanent error", async () => {
    const onRetry = jest.fn();
    render(
      <InlineError
        type="permanent"
        onRetry={onRetry}
      />
    );

    const retryBtn = screen.getByRole("button", { name: /Retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => expect(onRetry).toHaveBeenCalledTimes(1));
  });
});

// ---------------------------------------------------------------------------
// Test: 5 — onDismiss callback fires on dismiss button click
// ---------------------------------------------------------------------------

describe("InlineError — onDismiss callback", () => {
  it("fires onDismiss callback when dismiss button is clicked", async () => {
    const onDismiss = jest.fn();
    render(
      <InlineError
        type="transient"
        onRetry={jest.fn()}
        onDismiss={onDismiss}
      />
    );

    const dismissBtn = screen.getByRole("button", { name: /Dismiss error/i });
    fireEvent.click(dismissBtn);

    await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1));
  });

  it("does not render dismiss button when onDismiss is not provided", () => {
    render(
      <InlineError
        type="transient"
        onRetry={jest.fn()}
      />
    );

    expect(
      screen.queryByRole("button", { name: /Dismiss error/i })
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 6 — role="alert" and aria-live="assertive" are present
// ---------------------------------------------------------------------------

describe("InlineError — accessibility", () => {
  it("has role=alert and aria-live=assertive on transient error", () => {
    const { container } = render(
      <InlineError
        type="transient"
        onRetry={jest.fn()}
      />
    );

    const alertDiv = container.querySelector('[role="alert"]');
    expect(alertDiv).toBeInTheDocument();
    expect(alertDiv).toHaveAttribute("aria-live", "assertive");
  });

  it("has role=alert and aria-live=assertive on timeout error", () => {
    const { container } = render(
      <InlineError
        type="timeout"
        onRetry={jest.fn()}
      />
    );

    const alertDiv = container.querySelector('[role="alert"]');
    expect(alertDiv).toBeInTheDocument();
    expect(alertDiv).toHaveAttribute("aria-live", "assertive");
  });

  it("has role=alert and aria-live=assertive on permanent error", () => {
    const { container } = render(
      <InlineError
        type="permanent"
        onRetry={jest.fn()}
      />
    );

    const alertDiv = container.querySelector('[role="alert"]');
    expect(alertDiv).toBeInTheDocument();
    expect(alertDiv).toHaveAttribute("aria-live", "assertive");
  });
});

// ---------------------------------------------------------------------------
// Test: 7 — External links have rel="noopener noreferrer"
// ---------------------------------------------------------------------------

describe("InlineError — link security", () => {
  it("external links have rel=noopener noreferrer", () => {
    render(
      <InlineError
        type="permanent"
        onRetry={jest.fn()}
        logsSessionId="test-session-123"
      />
    );

    const logsLink = screen.getByRole("link", { name: /View session logs/i });
    expect(logsLink).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("links open in new tab (target=_blank)", () => {
    render(
      <InlineError
        type="permanent"
        onRetry={jest.fn()}
        logsSessionId="test-session-123"
      />
    );

    const logsLink = screen.getByRole("link", { name: /View session logs/i });
    expect(logsLink).toHaveAttribute("target", "_blank");
  });
});

// ---------------------------------------------------------------------------
// Test: 8 — customMessage overrides default body text
// ---------------------------------------------------------------------------

describe("InlineError — customMessage", () => {
  it("uses customMessage when provided instead of default body text", () => {
    const { container } = render(
      <InlineError
        type="transient"
        onRetry={jest.fn()}
        customMessage="Custom error message"
      />
    );

    expect(
      container.textContent?.includes("Custom error message")
    ).toBe(true);
    expect(
      container.textContent?.includes("Network error. The request could not be completed.")
    ).toBe(false);
  });

  it("uses default body text when customMessage is not provided", () => {
    const { container } = render(
      <InlineError
        type="transient"
        onRetry={jest.fn()}
      />
    );

    expect(
      container.textContent?.includes("Network error. The request could not be completed.")
    ).toBe(true);
  });
});
