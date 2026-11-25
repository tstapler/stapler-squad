import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntegrationCheckSection } from "./IntegrationCheckSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

const mockGetBacklogItem = jest.fn();
jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({ getBacklogItem: mockGetBacklogItem }),
}));

const child = (status: string, acStatus = "pending") =>
  ({ id: "child-1", status, acCriteria: [{ index: 0, text: "x", status: acStatus }] }) as unknown as BacklogItem;

describe("IntegrationCheckSection", () => {
  beforeEach(() => mockGetBacklogItem.mockReset());

  it("renders nothing when the item has no integration child", () => {
    const { container } = render(<IntegrationCheckSection item={{ status: "idea" }} />);
    expect(container).toBeEmptyDOMElement();
    expect(mockGetBacklogItem).not.toHaveBeenCalled();
  });

  it("shows Awaiting Integration Check while the child is in flight", async () => {
    mockGetBacklogItem.mockResolvedValue(child("in_progress"));
    render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} />);
    await waitFor(() => expect(mockGetBacklogItem).toHaveBeenCalledWith("child-1"));
    expect(screen.getByTestId("backlog-integration-check")).toHaveTextContent("Awaiting Integration Check");
  });

  it("shows Integration Passed when the child is done", async () => {
    mockGetBacklogItem.mockResolvedValue(child("done"));
    render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} />);
    await waitFor(() => expect(screen.getByTestId("backlog-integration-check")).toHaveTextContent("Integration Passed"));
  });

  it("shows Integration Failed when the child failed a criterion or was archived", async () => {
    mockGetBacklogItem.mockResolvedValue(child("review", "fail"));
    const { unmount } = render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} />);
    await waitFor(() => expect(screen.getByTestId("backlog-integration-check")).toHaveTextContent("Integration Failed"));
    unmount();

    mockGetBacklogItem.mockResolvedValue(child("archived"));
    render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} />);
    await waitFor(() => expect(screen.getByTestId("backlog-integration-check")).toHaveTextContent("Integration Failed"));
  });

  it("stays on Awaiting when the child cannot be loaded", async () => {
    mockGetBacklogItem.mockResolvedValue(null);
    render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} />);
    await waitFor(() => expect(mockGetBacklogItem).toHaveBeenCalled());
    expect(screen.getByTestId("backlog-integration-check")).toHaveTextContent("Awaiting Integration Check");
  });

  it("links to the integration child", async () => {
    mockGetBacklogItem.mockResolvedValue(child("in_progress"));
    const onItemClick = jest.fn();
    render(<IntegrationCheckSection item={{ status: "review", integrationChildId: "child-1" }} onItemClick={onItemClick} />);
    fireEvent.click(screen.getByText("View integration child"));
    expect(onItemClick).toHaveBeenCalledWith("child-1");
  });
});
