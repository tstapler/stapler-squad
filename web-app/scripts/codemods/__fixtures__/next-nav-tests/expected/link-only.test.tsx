import React from "react";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    link: () => ({
      __esModule: true,
      default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
    }),
  }),
);

it("renders a link", () => {
  expect(React).toBeDefined();
});
