import { PROGRAMS, getProgramDisplay, isKnownProgram, getPickerPrograms } from "./programs";

describe("PROGRAMS ordering", () => {
  it("places pi after claude and before aider", () => {
    const piIndex = PROGRAMS.findIndex((p) => p.value === "pi");
    const claudeIndex = PROGRAMS.findIndex((p) => p.value === "claude");
    const aiderIndex = PROGRAMS.findIndex((p) => p.value === "aider");
    expect(piIndex).toBeGreaterThan(claudeIndex);
    expect(piIndex).toBeLessThan(aiderIndex);
  });
});

describe("getProgramDisplay", () => {
  it("returns the friendly label for pi", () => {
    expect(getProgramDisplay("pi")).toBe("pi");
  });
});

describe("isKnownProgram", () => {
  it("returns true for pi regardless of the pi-support flag", () => {
    expect(isKnownProgram("pi")).toBe(true);
  });
});

describe("getPickerPrograms", () => {
  it("excludes pi when the flag is off", () => {
    const options = getPickerPrograms(false);
    expect(options.some((p) => p.value === "pi")).toBe(false);
  });

  it("includes pi at its PROGRAMS position when the flag is on", () => {
    const options = getPickerPrograms(true);
    expect(options).toEqual(PROGRAMS);
    expect(options.some((p) => p.value === "pi")).toBe(true);
  });
});
