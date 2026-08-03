import React from "react";
import { render, screen } from "@testing-library/react";
import { SourceSection } from "./SourceSection";

describe("SourceSection", () => {
  it("SourceSection_should_RenderIssueLinkAndLabelChips_When_PropsPresent", () => {
    render(
      <SourceSection
        externalUrl="https://github.com/acme/widget/issues/42"
        externalId="42"
        labels={["bug", "p1"]}
        defaultExpanded={true}
      />
    );

    const link = screen.getByRole("link", { name: /Issue #42/ });
    expect(link).toHaveAttribute("href", "https://github.com/acme/widget/issues/42");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");

    expect(screen.getByText("bug")).toBeInTheDocument();
    expect(screen.getByText("p1")).toBeInTheDocument();
  });

  it("SourceSection_should_OmitLabelChips_When_LabelsEmpty", () => {
    render(
      <SourceSection
        externalUrl="https://github.com/acme/widget/issues/42"
        externalId="42"
        labels={[]}
        defaultExpanded={true}
      />
    );

    expect(screen.getByRole("link", { name: /Issue #42/ })).toBeInTheDocument();
  });

  it("is collapsed by default", () => {
    render(
      <SourceSection
        externalUrl="https://github.com/acme/widget/issues/42"
        externalId="42"
        labels={[]}
        defaultExpanded={false}
      />
    );

    const header = screen.getByTestId("collapsible-header-source");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("link", { name: /Issue #42/ })).not.toBeInTheDocument();
  });
});
