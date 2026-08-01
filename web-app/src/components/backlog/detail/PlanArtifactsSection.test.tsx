import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { PlanArtifactsSection } from "./PlanArtifactsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

const getPlanArtifactContent = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({ getPlanArtifactContent }),
}));

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "ready",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

beforeEach(() => {
  getPlanArtifactContent.mockReset();
});

describe("PlanArtifactsSection", () => {
  it("renders nothing when there is no plan artifacts path", () => {
    const { container } = render(<PlanArtifactsSection item={makeItem()} defaultExpanded={false} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("is collapsed by default and reveals the path once expanded", () => {
    render(
      <PlanArtifactsSection
        item={makeItem({ planArtifactsPath: "/tmp/plans/item-1.md" })}
        defaultExpanded={false}
      />
    );

    const header = screen.getByTestId("collapsible-header-plan-artifacts");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("/tmp/plans/item-1.md")).not.toBeInTheDocument();
  });

  it("renders fetched markdown content", async () => {
    getPlanArtifactContent.mockResolvedValue({
      content: "# Plan\n\nDo the thing.",
      truncated: false,
      sizeBytes: 20n,
      modifiedAtUnixMs: 1000n,
    });

    render(
      <PlanArtifactsSection item={makeItem({ planArtifactsPath: "/tmp/plans/item-1.md" })} defaultExpanded={true} />
    );

    const rendered = await screen.findByTestId("backlog-plan-content-rendered");
    expect(rendered).toHaveTextContent("Do the thing.");
  });

  it("shows InlineError on fetch failure", async () => {
    getPlanArtifactContent.mockRejectedValue(new Error("network down"));

    render(
      <PlanArtifactsSection item={makeItem({ planArtifactsPath: "/tmp/plans/item-1.md" })} defaultExpanded={true} />
    );

    await waitFor(() => expect(screen.getByText(/network down/)).toBeInTheDocument());
  });

  it("shows a 'newer plan available' notice instead of silently swapping content on background re-fetch mtime drift, and Reload applies it", async () => {
    getPlanArtifactContent.mockResolvedValueOnce({
      content: "# Plan v1",
      truncated: false,
      sizeBytes: 10n,
      modifiedAtUnixMs: 1000n,
    });

    const { rerender } = render(
      <PlanArtifactsSection
        item={makeItem({ id: "item-1", planArtifactsPath: "/tmp/plans/item-1.md", updatedAt: "2026-08-01T00:00:00Z" })}
        defaultExpanded={true}
      />
    );
    await screen.findByTestId("backlog-plan-content-rendered");

    getPlanArtifactContent.mockResolvedValueOnce({
      content: "# Plan v2",
      truncated: false,
      sizeBytes: 10n,
      modifiedAtUnixMs: 2000n,
    });
    rerender(
      <PlanArtifactsSection
        item={makeItem({ id: "item-1", planArtifactsPath: "/tmp/plans/item-1.md", updatedAt: "2026-08-01T00:01:00Z" })}
        defaultExpanded={true}
      />
    );

    const notice = await screen.findByTestId("plan-content-stale-notice");
    expect(screen.getByTestId("backlog-plan-content-rendered")).toHaveTextContent("Plan v1");

    getPlanArtifactContent.mockResolvedValueOnce({
      content: "# Plan v2",
      truncated: false,
      sizeBytes: 10n,
      modifiedAtUnixMs: 2000n,
    });
    fireEvent.click(screen.getByText("Reload"));

    await waitFor(() => expect(screen.getByTestId("backlog-plan-content-rendered")).toHaveTextContent("Plan v2"));
    expect(notice).not.toBeInTheDocument();
  });

  it("ignores an older in-flight response that resolves after a newer one (out-of-order request guard)", async () => {
    let resolveFirst!: (v: unknown) => void;
    const firstCall = new Promise((resolve) => {
      resolveFirst = resolve;
    });
    getPlanArtifactContent.mockReturnValueOnce(firstCall);

    const { rerender } = render(
      <PlanArtifactsSection
        item={makeItem({ id: "item-1", planArtifactsPath: "/tmp/plans/item-1.md", updatedAt: "2026-08-01T00:00:00Z" })}
        defaultExpanded={true}
      />
    );

    // Second (newer) request fires and resolves before the first one does.
    getPlanArtifactContent.mockResolvedValueOnce({
      content: "# Newer Plan",
      truncated: false,
      sizeBytes: 12n,
      modifiedAtUnixMs: 2000n,
    });
    rerender(
      <PlanArtifactsSection
        item={makeItem({ id: "item-1", planArtifactsPath: "/tmp/plans/item-1.md", updatedAt: "2026-08-01T00:01:00Z" })}
        defaultExpanded={true}
      />
    );
    await screen.findByTestId("backlog-plan-content-rendered");
    expect(screen.getByTestId("backlog-plan-content-rendered")).toHaveTextContent("Newer Plan");

    // The stale first request finally resolves — must not overwrite the newer content.
    resolveFirst({ content: "# Stale Plan", truncated: false, sizeBytes: 11n, modifiedAtUnixMs: 1000n });
    await Promise.resolve();
    await Promise.resolve();

    expect(screen.getByTestId("backlog-plan-content-rendered")).toHaveTextContent("Newer Plan");
    expect(screen.queryByTestId("plan-content-stale-notice")).not.toBeInTheDocument();
  });
});
