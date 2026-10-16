import { render, screen } from "@testing-library/react";
import { DependenciesSection } from "./DependenciesSection";

const dep = (title: string, status: string) => ({ itemId: title, publicId: "", title, status });

describe("DependenciesSection", () => {
  it("renders nothing when the item has no dependencies", () => {
    const { container } = render(<DependenciesSection item={{}} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("lists blockers with status and marks resolved ones", () => {
    render(<DependenciesSection item={{ blockedBy: [dep("Open one", "queued"), dep("Shipped", "done")] }} />);
    const list = screen.getByTestId("backlog-blocked-by");
    expect(list).toHaveTextContent("Open one");
    expect(list).toHaveTextContent("(queued)");
    expect(list).toHaveTextContent("Shipped");
    expect(list).toHaveTextContent("(done, resolved)");
    expect(screen.queryByTestId("backlog-blocking")).not.toBeInTheDocument();
  });

  it("lists items this one blocks", () => {
    render(<DependenciesSection item={{ blocking: [dep("Waiting item", "queued")] }} />);
    expect(screen.getByTestId("backlog-blocking")).toHaveTextContent("Waiting item");
    expect(screen.queryByTestId("backlog-blocked-by")).not.toBeInTheDocument();
  });
});
