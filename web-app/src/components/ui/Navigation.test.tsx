import React from "react";
import { render, screen } from "@testing-library/react";
import { Navigation } from "./Navigation";

// Mock next/navigation
jest.mock("next/navigation", () => ({
  usePathname: jest.fn().mockReturnValue("/"),
}));

// Mock next/link so it renders as a plain anchor
jest.mock("next/link", () => {
  const MockLink = React.forwardRef<
    HTMLAnchorElement,
    React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; prefetch?: boolean }
  >(function MockLink({ href, children, prefetch: _prefetch, ...rest }, ref) {
    return (
      <a href={href} ref={ref} {...rest}>
        {children}
      </a>
    );
  });
  MockLink.displayName = "MockLink";
  return MockLink;
});

// Mock useFeatureFlag — default to false; overridden per-test
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: jest.fn().mockReturnValue(false),
}));

import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

describe("Navigation", () => {
  describe("Navigation_should_hide_backlogTab_when_featureFlagFalse", () => {
    it("Navigation > hides Backlog tab when feature flag is false", () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(false);
      render(<Navigation />);
      expect(screen.queryByText("Backlog")).toBeNull();
    });
  });

  describe("Navigation_should_show_backlogTab_when_featureFlagTrue", () => {
    it("Navigation > shows Backlog tab when feature flag is true", () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(true);
      render(<Navigation />);
      expect(screen.getByText("Backlog")).toBeInTheDocument();
    });
  });

  describe("Navigation_should_have_no_empty_navSlot_when_backlog_is_hidden", () => {
    it("Navigation > no empty nav slot when backlog is hidden", () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(false);
      const { container } = render(<Navigation />);
      const menuEl = container.querySelector('[role="menubar"]');
      expect(menuEl).not.toBeNull();
      // All rendered menu items should have text content (no empty slots)
      const items = Array.from(menuEl!.querySelectorAll('[role="none"]'));
      items.forEach((item) => {
        expect(item.textContent?.trim()).not.toBe("");
      });
    });
  });

  it("always renders Sessions and Review Queue links", () => {
    (useFeatureFlag as jest.Mock).mockReturnValue(false);
    render(<Navigation />);
    expect(screen.getByText("Sessions")).toBeInTheDocument();
    expect(screen.getByText("Review Queue")).toBeInTheDocument();
  });
});
