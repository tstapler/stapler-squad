/**
 * Tests for DiffRenderer's file-tree sidebar.
 *
 * Covers:
 *  - Single-file diff renders no sidebar (nothing to navigate)
 *  - Multi-file diff renders a sidebar entry per file
 *  - Clicking a sidebar entry scrolls to and highlights that file
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { DiffRenderer } from "../DiffRenderer";

const ONE_FILE_DIFF = `diff --git a/foo.go b/foo.go
index 0000000..1111111 100644
--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,2 @@
 package foo
+// added a comment
`;

const TWO_FILE_DIFF = `diff --git a/foo.go b/foo.go
index 0000000..1111111 100644
--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,2 @@
 package foo
+// added a comment
diff --git a/bar.go b/bar.go
index 2222222..3333333 100644
--- a/bar.go
+++ b/bar.go
@@ -1,1 +1,2 @@
 package bar
+// another comment
`;

describe("DiffRenderer", () => {
  it("DiffRenderer_should_omitSidebar_When_diffHasOneFile", () => {
    render(<DiffRenderer content={ONE_FILE_DIFF} added={1} removed={0} />);
    expect(screen.queryByRole("navigation", { name: /changed files/i })).toBeNull();
  });

  it("DiffRenderer_should_renderSidebarEntryPerFile_When_diffHasMultipleFiles", () => {
    render(<DiffRenderer content={TWO_FILE_DIFF} added={2} removed={0} />);
    const nav = screen.getByRole("navigation", { name: /changed files/i });
    expect(nav.querySelectorAll("button")).toHaveLength(2);
    expect(screen.getAllByText("foo.go").length).toBeGreaterThan(0);
    expect(screen.getAllByText("bar.go").length).toBeGreaterThan(0);
  });

  it("DiffRenderer_should_scrollToFile_When_sidebarEntryClicked", () => {
    render(<DiffRenderer content={TWO_FILE_DIFF} added={2} removed={0} />);
    const scrollIntoView = jest.fn();
    // jsdom doesn't implement scrollIntoView — stub it on every element.
    window.HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const nav = screen.getByRole("navigation", { name: /changed files/i });
    const barButton = Array.from(nav.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("bar.go")
    );
    expect(barButton).toBeTruthy();
    fireEvent.click(barButton!);

    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
  });

  it("DiffRenderer_should_renderDistinctErrorState_When_errorPropSet", () => {
    render(<DiffRenderer content="" added={0} removed={0} error="network error" />);
    expect(screen.getByText("Failed to load changes")).toBeInTheDocument();
    expect(screen.getByText("network error")).toBeInTheDocument();
    expect(screen.queryByText("No changes to display")).toBeNull();
  });

  it("DiffRenderer_should_callOnRefresh_When_retryButtonClicked", () => {
    const onRefresh = jest.fn();
    render(<DiffRenderer content="" added={0} removed={0} error="network error" onRefresh={onRefresh} />);
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it("DiffRenderer_should_renderEmptyState_When_noErrorAndDiffEmpty", () => {
    render(<DiffRenderer content="" added={0} removed={0} />);
    expect(screen.getByText("No changes to display")).toBeInTheDocument();
  });
});
