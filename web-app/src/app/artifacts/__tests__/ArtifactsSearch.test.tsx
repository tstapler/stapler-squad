import { fireEvent, screen, waitFor } from "@testing-library/react";
import { renderWithNav } from "@/lib/nav/testing";
import { artifactsApi } from "@/lib/artifacts/artifactsApi";
import { ArtifactsBrowser } from "@/components/artifacts/ArtifactsBrowser";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/contexts/NotificationContext", () => ({ useNotificationCommands: () => ({ addNotification: jest.fn() }) }));
jest.mock("@/lib/artifacts/artifactsApi", () => ({
  PAGE_SIZE: 50,
  artifactsApi: { browse: jest.fn(), search: jest.fn(), findByPr: jest.fn() },
}));

const api = artifactsApi as jest.Mocked<typeof artifactsApi>;

beforeEach(() => {
  jest.clearAllMocks();
  api.browse.mockResolvedValue({ rows: [], nextPageToken: "", totalCount: 0, unscanned: 0 });
  api.search.mockResolvedValue({ hits: [], unscanned: 0 });
});

async function type(text: string) {
  renderWithNav(<ArtifactsBrowser />, { initialEntries: ["/artifacts"] });
  await waitFor(() => expect(api.browse).toHaveBeenCalled());
  fireEvent.change(screen.getByTestId("artifacts-search"), { target: { value: text } });
}

describe("artifacts search box", () => {
  // U-150: search box should run at 2 chars, reject 1 char inline, and show the SHA hint for 2 to 6 hex chars
  it("search box should run at 2 chars, reject 1 char inline, and show the SHA hint for 2 to 6 hex chars", async () => {
    await type("a");
    expect(screen.getByTestId("artifacts-search-invalid")).toHaveTextContent("Enter at least 2 characters");
    fireEvent.click(screen.getByTestId("artifacts-search-submit"));
    expect(api.search).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId("artifacts-search"), { target: { value: "1a2b3c" } });
    expect(screen.queryByTestId("artifacts-search-invalid")).toBeNull();
    expect(screen.getByTestId("artifacts-search-sha-hint")).toHaveTextContent("Looks like a commit SHA. Use at least 7 characters to match a commit.");
    fireEvent.click(screen.getByTestId("artifacts-search-submit"));
    await waitFor(() => expect(api.search).toHaveBeenCalledTimes(1));
    expect(api.search.mock.calls[0][0].q).toBe("1a2b3c");
  });

  it("does not show the SHA hint for 7 or more hex characters or for non-hex text", async () => {
    await type("1a2b3c4");
    expect(screen.queryByTestId("artifacts-search-sha-hint")).toBeNull();
    fireEvent.change(screen.getByTestId("artifacts-search"), { target: { value: "zebra" } });
    expect(screen.queryByTestId("artifacts-search-sha-hint")).toBeNull();
  });

  it("Escape clears the box and returns to browse", async () => {
    await type("zebra");
    fireEvent.click(screen.getByTestId("artifacts-search-submit"));
    await waitFor(() => expect(api.search).toHaveBeenCalled());
    fireEvent.keyDown(screen.getByTestId("artifacts-search"), { key: "Escape" });
    await waitFor(() => expect(screen.getByTestId("artifacts-search")).toHaveValue(""));
    expect(screen.queryByTestId("artifacts-clear-search")).toBeNull();
  });
});
