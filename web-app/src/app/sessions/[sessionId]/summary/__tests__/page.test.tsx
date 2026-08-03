/**
 * Route smoke test for the durable standalone `/sessions/[sessionId]/summary`
 * route (Epic 3.3, Task 3.3.1b). Confirms:
 *  - the page reads `sessionId` from the route param and passes it straight
 *    through to SessionSummaryPanel
 *  - the page renders with no Redux Provider in the tree — if the page (not
 *    just SessionSummaryPanel, which is mocked out below) ever called
 *    `useAppSelector`, react-redux would throw "could not find react-redux
 *    context value" since there's no <Provider> here, so a clean render is
 *    itself proof this route has no Redux sessions-list dependency.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import SessionSummaryPage from "../page";

const mockUseParams = jest.fn();
jest.mock("next/navigation", () => ({
  useParams: () => mockUseParams(),
}));

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: () => {},
}));

const mockSessionSummaryPanel = jest.fn((_props: { sessionId: string }) => (
  <div data-testid="mock-session-summary-panel" />
));
jest.mock("@/components/sessions/SessionSummaryPanel", () => ({
  SessionSummaryPanel: (props: { sessionId: string }) => mockSessionSummaryPanel(props),
}));

describe("SessionSummaryPage", () => {
  beforeEach(() => {
    mockSessionSummaryPanel.mockClear();
    mockUseParams.mockReturnValue({ sessionId: "sess-123" });
  });

  it("renders SessionSummaryPanel with the sessionId taken from the route param", () => {
    render(<SessionSummaryPage />);

    expect(mockSessionSummaryPanel).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "sess-123" }),
    );
    expect(screen.getByTestId("mock-session-summary-panel")).toBeInTheDocument();
  });

  it("renders a back link and no Redux-dependent chrome (terminal/VCS/files tabs)", () => {
    render(<SessionSummaryPage />);

    expect(screen.getByText("← Back")).toBeInTheDocument();
    // Renders without a <Provider> in the tree at all — see file header comment.
  });

  it("handles a route param arriving as an array (Next.js catch-all edge case) by using the first segment", () => {
    mockUseParams.mockReturnValue({ sessionId: ["sess-456"] });

    render(<SessionSummaryPage />);

    expect(mockSessionSummaryPanel).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "sess-456" }),
    );
  });
});
