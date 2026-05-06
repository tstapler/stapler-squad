import { parseInputWithSeparator } from "./parseInput";

describe("parseInputWithSeparator", () => {
  it("slugifies name and returns empty prompt when no separator", () => {
    const result = parseInputWithSeparator("my feature");
    expect(result.name).toBe("my-feature");
    expect(result.firstPrompt).toBe("");
  });

  it("splits on first > and slugifies the name part", () => {
    const result = parseInputWithSeparator("my feature > implement auth");
    expect(result.name).toBe("my-feature");
    expect(result.firstPrompt).toBe("implement auth");
  });

  it("keeps everything after first > (including nested >) as firstPrompt", () => {
    const result = parseInputWithSeparator("a > b > c");
    expect(result.name).toBe("a");
    expect(result.firstPrompt).toBe("b > c");
  });

  it("returns empty name when separator is at start", () => {
    const result = parseInputWithSeparator("> implement auth");
    expect(result.name).toBe("");
    expect(result.firstPrompt).toBe("implement auth");
  });

  it("returns empty prompt when separator is at end", () => {
    const result = parseInputWithSeparator("my feature >");
    expect(result.name).toBe("my-feature");
    expect(result.firstPrompt).toBe("");
  });

  it("trims whitespace from firstPrompt", () => {
    const result = parseInputWithSeparator("feature >   do something   ");
    expect(result.firstPrompt).toBe("do something");
  });

  it("handles empty input", () => {
    const result = parseInputWithSeparator("");
    expect(result.name).toBe("");
    expect(result.firstPrompt).toBe("");
  });
});
