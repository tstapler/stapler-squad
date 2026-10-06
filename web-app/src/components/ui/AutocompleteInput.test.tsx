import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { AutocompleteInput } from "./AutocompleteInput";

describe("AutocompleteInput default filter (program, repo path, git branch consumers)", () => {
  it("filters suggestions by plain case-insensitive substring match when no filterFn is passed", () => {
    const onChange = jest.fn();
    render(
      <AutocompleteInput
        id="test"
        value="cla"
        onChange={onChange}
        suggestions={["claude", "aider", "opencode", "bash"]}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));

    expect(screen.getByRole("option", { name: "claude" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "aider" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "opencode" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "bash" })).not.toBeInTheDocument();
  });

  it("renders each suggestion's raw string as its label when no getLabel is passed", () => {
    const onChange = jest.fn();
    render(
      <AutocompleteInput
        id="test"
        value="feature"
        onChange={onChange}
        suggestions={["feature/foo", "feature/bar"]}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));

    expect(screen.getByText("feature/foo")).toBeInTheDocument();
    expect(screen.getByText("feature/bar")).toBeInTheDocument();
  });

  it("sets the raw suggestion string on click when no filterFn/getLabel is passed", () => {
    const onChange = jest.fn();
    render(
      <AutocompleteInput
        id="test"
        value="/Users/tstapler/proj"
        onChange={onChange}
        suggestions={["/Users/tstapler/projects/foo", "/Users/tstapler/projects/bar"]}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));
    fireEvent.click(screen.getByText("/Users/tstapler/projects/foo"));

    expect(onChange).toHaveBeenCalledWith("/Users/tstapler/projects/foo");
  });
});

describe("AutocompleteInput filterFn/getLabel overrides", () => {
  it("uses filterFn to rank/filter suggestions instead of the default substring match", () => {
    const onChange = jest.fn();
    const filterFn = (query: string, suggestions: string[]) =>
      // Reverse the default order so we can prove filterFn's output order is honored.
      suggestions.filter((s) => s.includes(query)).reverse();

    render(
      <AutocompleteInput
        id="test"
        value="a"
        onChange={onChange}
        suggestions={["a1", "a2"]}
        filterFn={filterFn}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["a2", "a1"]);
  });

  it("uses getLabel to render a display label distinct from the underlying value", () => {
    const onChange = jest.fn();
    render(
      <AutocompleteInput
        id="test"
        value=""
        onChange={onChange}
        suggestions={["family:sonnet"]}
        getLabel={(v) => (v === "family:sonnet" ? "Sonnet (latest)" : v)}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));

    expect(screen.getByText("Sonnet (latest)")).toBeInTheDocument();
    expect(screen.queryByText("family:sonnet")).not.toBeInTheDocument();
  });

  it("clicking a labeled suggestion still commits the underlying value, not the label", () => {
    const onChange = jest.fn();
    render(
      <AutocompleteInput
        id="test"
        value=""
        onChange={onChange}
        suggestions={["family:sonnet"]}
        getLabel={(v) => (v === "family:sonnet" ? "Sonnet (latest)" : v)}
      />
    );

    fireEvent.focus(screen.getByRole("textbox"));
    fireEvent.click(screen.getByText("Sonnet (latest)"));

    expect(onChange).toHaveBeenCalledWith("family:sonnet");
  });
});
