/**
 * Focus-restoration regression test for DebugMenu (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap so the real trap-and-restore
 * behavior runs end to end. Header.tsx's debug button captures its trigger
 * via the click event's `currentTarget` in `debugButtonTriggerRef`.
 */

import React, { useRef, useState } from "react";
import type { MouseEvent } from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import { DebugMenu } from "../DebugMenu";

jest.mock("../DebugMenu.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));
jest.mock("@/gen/session/v1/session_pb", () => ({ SessionService: {} }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));
jest.mock("@/lib/utils/notifications", () => ({
  getNotificationPreference: () => true,
  setNotificationPreference: jest.fn(),
  requestNotificationPermission: jest.fn(),
}));

global.fetch = jest.fn(() =>
  Promise.resolve({ json: () => Promise.resolve({ level: "INFO" }) })
) as jest.Mock;

function Harness() {
  const [isOpen, setIsOpen] = useState(false);
  const debugButtonTriggerRef = useRef<HTMLElement | null>(null);

  return (
    <>
      <button
        data-testid="debug-menu-button"
        onClick={(event: MouseEvent<HTMLButtonElement>) => {
          debugButtonTriggerRef.current = event.currentTarget;
          setIsOpen(true);
        }}
      >
        Debug
      </button>
      <DebugMenu
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        triggerRef={debugButtonTriggerRef}
      />
    </>
  );
}

describe("DebugMenu focus restoration", () => {
  it("DebugMenu_should_restoreFocusToDebugButton_When_closedViaCloseButton", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("debug-menu-button");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByLabelText("Close debug menu"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("DebugMenu_should_restoreFocusToDebugButton_When_closedViaDoneButton", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("debug-menu-button");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByText("Done"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
