// @feature ui:tcc-enrichment-consent
import React from "react";
import { fireEvent, act, render, screen, waitFor, within } from "@testing-library/react";
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

it("keeps mandatory text unclamped with long names at narrow widths", async () => {
 mockStatus.mockResolvedValue({ enabled: true, roots: [{ ...root, sessionTitles: ["Long session name ".repeat(30)] }] });
 render(<div style={{ width: 320 }}><TCCEnrichmentBanner /></div>);
 const message = await screen.findByText(/git metadata/);
 expect(message.className).not.toContain("textClamp");
 expect(message.textContent).toContain("app consent is not an OS grant");
});

it("keeps a save error scoped to its root", async () => {
 mockStatus.mockResolvedValue({ enabled: true, roots: [root, { ...root, id: "other", root: "/other" }] });
 mockContinue.mockRejectedValue(new Error("disk full"));
 render(<TCCEnrichmentBanner />);
 const banners = await screen.findAllByTestId("tcc-enrichment-banner");
 fireEvent.click(within(banners[0]).getByRole("button", { name: "Continue to macOS permission request" }));
 expect(await within(banners[0]).findByText(/disk full/)).toBeInTheDocument();
 expect(within(banners[1]).queryByText(/disk full/)).not.toBeInTheDocument();
});

it("reports refresh failure after successful consent without offering consent again", async () => {
 mockStatus.mockResolvedValueOnce({ enabled: true, roots: [root] }).mockRejectedValue(new Error("offline"));
 render(<TCCEnrichmentBanner />);
 fireEvent.click(await screen.findByRole("button", { name: "Continue to macOS permission request" }));
 expect(await screen.findByText(/Consent saved; enrichment may already be running/)).toBeInTheDocument();
 expect(screen.queryByText(/Could not continue/)).not.toBeInTheDocument();
 expect(screen.queryByRole("button", { name: "Continue to macOS permission request" })).not.toBeInTheDocument();
});

it("keeps acknowledged consent when an older poll resolves afterward", async () => {
 let resolveOld!: (value: unknown) => void;
 const old = new Promise((resolve) => { resolveOld = resolve; });
 mockStatus.mockResolvedValueOnce({ enabled: true, roots: [root] }).mockReturnValueOnce(old).mockRejectedValue(new Error("offline"));
 const interval = jest.spyOn(global, "setInterval");
 render(<TCCEnrichmentBanner />);
 await screen.findByRole("button", { name: "Continue to macOS permission request" });
 await act(async () => { (interval.mock.calls[0][0] as () => void)(); });
 fireEvent.click(screen.getByRole("button", { name: "Continue to macOS permission request" }));
 await screen.findByText(/Consent saved; enrichment may already be running/);
 await act(async () => { resolveOld({ enabled: true, roots: [root] }); });
 expect(screen.queryByRole("button", { name: "Continue to macOS permission request" })).not.toBeInTheDocument();
 expect(screen.getByText(/Consent saved; enrichment may already be running/)).toBeInTheDocument();
 interval.mockRestore();
});

it("does not render on other platforms or when disabled", async () => {
 mockStatus.mockResolvedValue({ enabled: false, roots: [] });
 const { container } = render(<TCCEnrichmentBanner />);
 await waitFor(() => expect(mockStatus).toHaveBeenCalled());
 expect(container).toBeEmptyDOMElement();
});

it("links to System Settings when macOS denied access", async () => {
 const settingsUrl = "x-apple.systempreferences:com.apple.preference.security?Privacy_FilesAndFolders";
 mockStatus.mockResolvedValue({ enabled: true, roots: [{ ...root, appConsented: true, outcome: "denied", settingsUrl }] });
 render(<TCCEnrichmentBanner />);
 expect(await screen.findByTestId("tcc-settings-link")).toHaveAttribute("href", settingsUrl);
 expect(screen.getByTestId("tcc-settings-link")).toHaveAttribute("rel", "noreferrer noopener");
});

it("re-checks a denied root through the consent RPC", async () => {
 mockStatus.mockResolvedValue({ enabled: true, roots: [{ ...root, appConsented: true, outcome: "denied", settingsUrl: "x-apple.systempreferences:a" }] });
 render(<TCCEnrichmentBanner />);
 fireEvent.click(await screen.findByRole("button", { name: "Check again" }));
 await waitFor(() => expect(mockContinue).toHaveBeenCalledWith({ rootId: "opaque-id" }));
});
