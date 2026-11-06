import React from "react";
import { usePathname } from "next/navigation";

jest.mock("next/navigation", () => ({
  usePathname: jest.fn(),
}));

// Mock next/link as a plain anchor
jest.mock("next/link", () => {
  const MockLink = ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>;
  return MockLink;
});

it("uses both", () => {
  expect(usePathname).toBeDefined();
});
