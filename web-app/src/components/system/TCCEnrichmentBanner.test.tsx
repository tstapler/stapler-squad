// @feature ui:tcc-enrichment-consent
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TCCEnrichmentBanner } from "./TCCEnrichmentBanner";

const mockStatus = jest.fn();
const mockContinue = jest.fn();
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({ getTCCEnrichmentStatus: mockStatus, continueTCCEnrichment: mockContinue }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: jest.fn() }));
jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));

const root = { id: "opaque-id", root: "/protected", paths: ["/protected/repo"], sessionTitles: ["My session"], appConsented: false, outcome: "pending" };
beforeEach(() => { jest.clearAllMocks(); mockStatus.mockResolvedValue({ enabled: true, roots: [root] }); mockContinue.mockResolvedValue({}); });

it("explains enrichment before sending consent and supports deferring", async () => {
 render(<TCCEnrichmentBanner />);
 expect(await screen.findByText(/My session/)).toBeInTheDocument();
 expect(screen.getByText(/git metadata/)).toBeInTheDocument();
 expect(mockContinue).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button", { name: "Not now" }));
 expect(screen.queryByRole("button", { name: "Continue to macOS permission request" })).not.toBeInTheDocument();
 expect(mockContinue).not.toHaveBeenCalled();
});

it("sends only the server root ID after the readiness click", async () => {
 render(<TCCEnrichmentBanner />);
 fireEvent.click(await screen.findByRole("button", { name: "Continue to macOS permission request" }));
 await waitFor(() => expect(mockContinue).toHaveBeenCalledWith({ rootId: "opaque-id" }));
});

it("shows save failures without claiming an OS denial", async () => {
 mockContinue.mockRejectedValue(new Error("disk full"));
 render(<TCCEnrichmentBanner />);
 fireEvent.click(await screen.findByRole("button", { name: "Continue to macOS permission request" }));
 expect(await screen.findByText(/disk full/)).toBeInTheDocument();
 expect(screen.getByRole("button", { name: "Continue to macOS permission request" })).toBeEnabled();
});

it("labels a slow result without asserting an OS prompt", async () => {
 mockStatus.mockResolvedValue({ enabled: true, roots: [{ ...root, appConsented: true, outcome: "unresolved/slow" }] });
 render(<TCCEnrichmentBanner />);
 expect(await screen.findByText(/unresolved\/slow/)).toBeInTheDocument();
 expect(screen.queryByRole("button", { name: "Continue to macOS permission request" })).not.toBeInTheDocument();
});

it("does not render on other platforms or when disabled", async () => {
 mockStatus.mockResolvedValue({ enabled: false, roots: [] });
 const { container } = render(<TCCEnrichmentBanner />);
 await waitFor(() => expect(mockStatus).toHaveBeenCalled());
 expect(container).toBeEmptyDOMElement();
});
