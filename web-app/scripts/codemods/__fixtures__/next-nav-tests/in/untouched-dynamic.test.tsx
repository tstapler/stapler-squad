// next/dynamic mocks stay valid until Task 9.1b; the codemod must not touch them.
jest.mock("next/dynamic", () => () => () => null);

it("is left alone", () => {
  expect(true).toBe(true);
});
