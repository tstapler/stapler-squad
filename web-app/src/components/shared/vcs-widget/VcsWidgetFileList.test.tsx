import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VcsWidgetFileList } from "./VcsWidgetFileList";
import type { FileChangeSummary } from "@/lib/vcs/types";

function makeFile(overrides: Partial<FileChangeSummary> = {}): FileChangeSummary {
  return {
    path: "src/foo.ts",
    status: "modified",
    additions: 5,
    deletions: 2,
    section: "unstaged",
    ...overrides,
  };
}

describe("VcsWidgetFileList", () => {
  it("VcsWidgetFileList_should_RenderNativeButtonReachableByTab_When_OnNavigateToFileProvided", async () => {
    const onNavigateToFile = jest.fn();
    render(<VcsWidgetFileList fileChanges={[makeFile()]} onNavigateToFile={onNavigateToFile} />);

    const button = screen.getByRole("button", { name: /src\/foo\.ts/ });
    expect(button.tagName).toBe("BUTTON");

    const user = userEvent.setup();
    await user.tab();
    expect(button).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(onNavigateToFile).toHaveBeenCalledWith("src/foo.ts");
  });

  it("VcsWidgetFileList_should_RenderPlainSpanNotDeadButton_When_OnNavigateToFileOmitted", () => {
    render(<VcsWidgetFileList fileChanges={[makeFile()]} />);
    expect(screen.queryByRole("button", { name: /src\/foo\.ts/ })).not.toBeInTheDocument();
    const span = screen.getByText("src/foo.ts");
    expect(span.closest("span")).not.toBeNull();
  });

  it("shows conflicts fully and caps other sections at 20 with a Show all button, conflicts heading first", () => {
    const conflicts: FileChangeSummary[] = Array.from({ length: 2 }, (_, i) =>
      makeFile({ path: `conflict-${i}.ts`, section: "conflict", status: "conflict" })
    );
    const unstaged: FileChangeSummary[] = Array.from({ length: 60 }, (_, i) =>
      makeFile({ path: `unstaged-${i}.ts`, section: "unstaged" })
    );

    render(<VcsWidgetFileList fileChanges={[...conflicts, ...unstaged]} />);

    expect(screen.getByText("Conflicts (2)")).toBeInTheDocument();
    expect(screen.getByText("Unstaged Changes (60)")).toBeInTheDocument();
    conflicts.forEach((f) => expect(screen.getByText(f.path)).toBeInTheDocument());

    // Only the first 20 unstaged files render before expansion.
    expect(screen.getByText("unstaged-19.ts")).toBeInTheDocument();
    expect(screen.queryByText("unstaged-20.ts")).not.toBeInTheDocument();

    const showAllButton = screen.getByRole("button", { name: "Show all 60 files" });
    expect(showAllButton).toBeInTheDocument();

    const headings = screen.getAllByRole("heading", { level: 4 }).map((h) => h.textContent);
    expect(headings.indexOf("Conflicts (2)")).toBeLessThan(headings.indexOf("Unstaged Changes (60)"));
  });

  it("expands a capped section on Show all click", async () => {
    const unstaged: FileChangeSummary[] = Array.from({ length: 25 }, (_, i) =>
      makeFile({ path: `unstaged-${i}.ts`, section: "unstaged" })
    );
    render(<VcsWidgetFileList fileChanges={unstaged} />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Show all 25 files" }));

    expect(screen.getByText("unstaged-24.ts")).toBeInTheDocument();
  });

  it("hides a section entirely when it has no files", () => {
    render(<VcsWidgetFileList fileChanges={[makeFile({ section: "staged" })]} />);
    expect(screen.queryByText(/Untracked Files/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Conflicts/)).not.toBeInTheDocument();
  });
});
