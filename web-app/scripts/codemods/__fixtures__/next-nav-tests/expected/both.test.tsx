import React from "react";
import { usePathname } from "@/lib/nav";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    navigation: () => ({
      usePathname: jest.fn(),
    }),
    link: () => {
      const MockLink = ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>;
      return MockLink;
    },
  }),
);

// Mock next/link as a plain anchor
it("uses both", () => {
  expect(usePathname).toBeDefined();
});
