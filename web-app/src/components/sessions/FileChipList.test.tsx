import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { FileChipList, type AttachedFile } from "./FileChipList";

function makeFile(name: string, type: string): File {
  return new File(["x"], name, { type });
}

function makeAttached(name: string, type: string, previewUrl?: string): AttachedFile {
  const file = makeFile(name, type);
  return { file, path: `/tmp/paste-${name}`, previewUrl, name, size: file.size };
}

describe("FileChipList", () => {
  it("renders nothing when files array is empty", () => {
    const { container } = render(<FileChipList files={[]} onRemove={jest.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders a chip for each file", () => {
    render(
      <FileChipList
        files={[makeAttached("report.pdf", "application/pdf"), makeAttached("script.py", "text/x-python")]}
        onRemove={jest.fn()}
      />
    );
    expect(screen.getByText("report.pdf")).toBeInTheDocument();
    expect(screen.getByText("script.py")).toBeInTheDocument();
  });

  it("shows thumbnail img for image files with previewUrl", () => {
    render(
      <FileChipList
        files={[makeAttached("photo.png", "image/png", "blob:fake-url")]}
        onRemove={jest.fn()}
      />
    );
    const img = screen.getByRole("img", { name: "photo.png" });
    expect(img).toHaveAttribute("src", "blob:fake-url");
  });

  it("does not show img element for non-image files", () => {
    render(
      <FileChipList
        files={[makeAttached("archive.zip", "application/zip")]}
        onRemove={jest.fn()}
      />
    );
    expect(screen.queryByRole("img", { hidden: true })).toBeNull();
  });

  it("calls onRemove with correct index when × is clicked", () => {
    const onRemove = jest.fn();
    render(
      <FileChipList
        files={[makeAttached("a.txt", "text/plain"), makeAttached("b.json", "application/json")]}
        onRemove={onRemove}
      />
    );
    fireEvent.click(screen.getByLabelText("Remove b.json"));
    expect(onRemove).toHaveBeenCalledWith(1);
  });

  it("remove buttons have accessible aria-label", () => {
    render(
      <FileChipList
        files={[makeAttached("report.pdf", "application/pdf")]}
        onRemove={jest.fn()}
      />
    );
    expect(screen.getByLabelText("Remove report.pdf")).toBeInTheDocument();
  });

  it("chip list has accessible role and label", () => {
    render(
      <FileChipList
        files={[makeAttached("file.txt", "text/plain")]}
        onRemove={jest.fn()}
      />
    );
    expect(screen.getByRole("list", { name: "Attached files" })).toBeInTheDocument();
  });
});
