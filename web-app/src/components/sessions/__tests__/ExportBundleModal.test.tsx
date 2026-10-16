import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ExportBundleModal, CONSENT_TEXT } from "../ExportBundleModal";

const download = jest.fn();
const saveOnServer = jest.fn();
const createBacklogItem = jest.fn();
const showActionToast = jest.fn();

jest.mock("@/lib/hooks/useExportBundle", () => ({
  useExportBundle: () => ({ download, saveOnServer, loading: false, error: null }),
}));
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({ createBacklogItem }),
}));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ showActionToast }),
}));

const props = { open: true, onOpenChange: jest.fn(), sessionId: "s1", sessionTitle: "Fix login", repoPath: "/repo" };

beforeEach(() => jest.clearAllMocks());

describe("ExportBundleModal", () => {
  it("disables Export until consent is given", () => {
    render(<ExportBundleModal {...props} />);
    expect(screen.getByText(CONSENT_TEXT)).toBeInTheDocument();
    expect(screen.getByTestId("export-bundle-submit")).toBeDisabled();
    fireEvent.click(screen.getByTestId("export-bundle-consent"));
    expect(screen.getByTestId("export-bundle-submit")).toBeEnabled();
  });

  it("downloads and toasts success", async () => {
    download.mockResolvedValue(true);
    render(<ExportBundleModal {...props} />);
    fireEvent.click(screen.getByTestId("export-bundle-consent"));
    fireEvent.click(screen.getByTestId("export-bundle-submit"));
    await waitFor(() => expect(showActionToast).toHaveBeenCalledWith("Bundle downloaded.", "success", "bundle-export-s1"));
    expect(download).toHaveBeenCalledWith("s1", "Fix login");
  });

  it("shows an alert and error toast on failure", async () => {
    download.mockResolvedValue(false);
    render(<ExportBundleModal {...props} />);
    fireEvent.click(screen.getByTestId("export-bundle-consent"));
    fireEvent.click(screen.getByTestId("export-bundle-submit"));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(showActionToast).toHaveBeenCalledWith("Bundle export failed.", "error", "bundle-export-s1");
  });

  it("saves on server then creates a backlog item referencing the bundle hash", async () => {
    saveOnServer.mockResolvedValue({ filePath: "/data/bundles/fix.zip", contentHash: "abc123", fileSizeBytes: BigInt(1) });
    createBacklogItem.mockResolvedValue({ item: { id: "item-9" }, triageTriggered: false });
    render(<ExportBundleModal {...props} />);
    fireEvent.click(screen.getByLabelText("Save on server"));
    fireEvent.click(screen.getByTestId("export-bundle-consent"));
    fireEvent.click(screen.getByTestId("export-bundle-submit"));
    fireEvent.click(await screen.findByTestId("create-backlog-from-bundle"));
    await waitFor(() => expect(createBacklogItem).toHaveBeenCalled());
    const arg = createBacklogItem.mock.calls[0][0];
    expect(arg.title).toMatch(/^Diagnostic bundle: Fix login \(\d{8}\)$/);
    expect(arg.notes).toBe("Bundle: fix.zip (SHA-256: abc123)");
    expect(arg.notes).not.toContain("/data");
    expect(await screen.findByText("View backlog item →")).toBeInTheDocument();
  });
});
