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

    const link = screen.getByRole("link", { name: "Issue acme/widget#42" });
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

    expect(screen.getByRole("link", { name: "Issue acme/widget#42" })).toBeInTheDocument();
  });

  it("SourceSection_should_FallBackToBareId_When_ExternalUrlUnparseable", () => {
    render(
      <SourceSection externalUrl="https://example.com/tracker/42" externalId="42" labels={[]} defaultExpanded={true} />
    );

    expect(screen.getByRole("link", { name: "Issue #42" })).toBeInTheDocument();
  });

  it("SourceSection_should_BeCollapsed_When_DefaultExpandedFalse", () => {
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
    expect(screen.queryByRole("link", { name: /Issue acme\/widget#42/ })).not.toBeInTheDocument();
  });

  it("SourceSection_should_OmitIssueNumber_When_ExternalIdMissing", () => {
    render(
      <SourceSection
        externalUrl="https://github.com/acme/widget/issues/42"
        externalId={undefined}
        labels={[]}
        defaultExpanded={true}
      />
    );

    // No literal "Issue #undefined" — falls back to a bare "Issue" label,
    // but the link itself (still the real provenance URL) is preserved.
    const link = screen.getByRole("link", { name: "Issue" });
    expect(link).toHaveAttribute("href", "https://github.com/acme/widget/issues/42");
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });
});

describe("SourceSection URL safety", () => {
  it("renders no link for a javascript: externalUrl", () => {
    const { container } = render(
      <SourceSection externalUrl="javascript:alert(1)" externalId="42" labels={[]} defaultExpanded={true} />
    );
    expect(container.querySelector("a")).toBeNull();
    expect(screen.getByText("Issue #42")).toBeInTheDocument();
  });
});

const base = { externalUrl: "https://github.com/o/r/issues/1", externalId: "1", labels: [], defaultExpanded: true };

describe("SourceSection working host", () => {
  it("shows the claiming host when set", () => {
    render(<SourceSection {...base} claimingHostId="host_01K" />);
    expect(screen.getByTestId("source-working-host")).toHaveTextContent("Working host: host_01K");
  });

  it("shows nothing when there is no claiming host", () => {
    render(<SourceSection {...base} />);
    expect(screen.queryByTestId("source-working-host")).toBeNull();
  });
});
