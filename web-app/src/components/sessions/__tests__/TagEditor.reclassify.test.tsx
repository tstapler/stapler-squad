/**
 * Manual AI re-classification from TagEditor: the re-run button is hidden
 * without a sessionId, calls ReclassifySessionTags on click, refreshes the
 * dialog tags from the server-applied result, and surfaces failures.
 */

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TagEditor } from "../TagEditor";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

const mockReclassify = jest.fn();

beforeEach(() => {
  jest.clearAllMocks();
  mockReclassify.mockResolvedValue({ tags: ["Feature"] });
  (createClient as jest.Mock).mockReturnValue({
    reclassifySessionTags: mockReclassify,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

function renderEditor(props?: Partial<Parameters<typeof TagEditor>[0]>) {
  const onReclassified = jest.fn();
  render(
    <TagEditor
      tags={["Unclassified"]}
      onSave={jest.fn()}
      onCancel={jest.fn()}
      sessionTitle="My Session"
      sessionId="sess-1"
      onReclassified={onReclassified}
      {...props}
    />,
  );
  return { onReclassified };
}

describe("TagEditor — manual re-classification", () => {
  it("hides the re-run button when no sessionId is provided", () => {
    render(
      <TagEditor
        tags={["Bugfix"]}
        onSave={jest.fn()}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />,
    );
    expect(
      screen.queryByTestId("tag-reclassify-button"),
    ).toBeNull();
  });

  it("calls ReclassifySessionTags and refreshes tags on success", async () => {
    const { onReclassified } = renderEditor();

    fireEvent.click(screen.getByTestId("tag-reclassify-button"));
    expect(await screen.findByText("Classifying…")).not.toBeNull();

    await waitFor(() =>
      expect(mockReclassify).toHaveBeenCalledWith({ sessionId: "sess-1" }),
    );
    expect(await screen.findByText("AI classification applied: Feature")).not.toBeNull();
    expect(onReclassified).toHaveBeenCalledWith(["Feature"]);
    expect(screen.getByText("Feature")).not.toBeNull();
  });

  it("surfaces re-classification failures without closing", async () => {
    mockReclassify.mockRejectedValueOnce(new Error("pool down"));
    renderEditor();

    fireEvent.click(screen.getByTestId("tag-reclassify-button"));
    expect(
      await screen.findByText("Re-classification failed: pool down"),
    ).not.toBeNull();
    // Dialog stays open with the original tags intact.
    expect(screen.getByText("Unclassified")).not.toBeNull();
  });
});
