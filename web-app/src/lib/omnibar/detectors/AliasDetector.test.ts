import { AliasDetector } from "./AliasDetector";
import { InputType } from "../types";
import type { AliasEntry } from "../../hooks/useAliases";
import { SessionType } from "@/gen/session/v1/types_pb";

const mockAliases: AliasEntry[] = [
  {
    name: "myproj",
    group: "work",
    path: "~/code/myproj",
    description: "My project",
    profile: "",
    program: "claude",
    autoYes: false,
    tags: [],
    sessionType: SessionType.UNSPECIFIED,
  },
  {
    name: "quick",
    group: "tools",
    path: "",
    description: "Quick haiku session",
    profile: "",
    program: "claude",
    autoYes: false,
    tags: [],
    sessionType: SessionType.UNSPECIFIED,
  },
];

describe("AliasDetector", () => {
  let detector: AliasDetector;

  beforeEach(() => {
    detector = new AliasDetector(mockAliases);
  });

  // T-UNIT-TS-039
  it("AliasDetector_should_havePriority36", () => {
    expect(detector.priority).toBe(36);
  });

  // T-UNIT-TS-036
  it("AliasDetector_should_returnAliasBrowse_When_bareAtSign", () => {
    const result = detector.detect("@");
    expect(result).not.toBeNull();
    expect(result?.type).toBe(InputType.AliasBrowse);
  });

  // T-UNIT-TS-037
  it("AliasDetector_should_returnAliasBrowse_When_partialNameNoSpace", () => {
    const result = detector.detect("@myp");
    expect(result).not.toBeNull();
    expect(result?.type).toBe(InputType.AliasBrowse);
    expect(result?.metadata?.partial).toBe("myp");
  });

  // T-UNIT-TS-030
  it("AliasDetector_should_returnAlias_When_knownAliasWithTrailingSpace", () => {
    const result = detector.detect("@myproj ");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.aliasName).toBe("myproj");
  });

  // T-UNIT-TS-031
  it("AliasDetector_should_returnAlias_When_knownAliasWithBranch", () => {
    const result = detector.detect("@myproj:feature/auth ");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.branch).toBe("feature/auth");
  });

  // T-UNIT-TS-032
  it("AliasDetector_should_returnAlias_When_knownAliasWithLabel", () => {
    const result = detector.detect("@myproj working on auth");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.label).toBe("working on auth");
  });

  // T-UNIT-TS-033
  it("AliasDetector_should_returnAlias_When_knownAliasWithExtraFlags", () => {
    const result = detector.detect("@myproj --model haiku");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.extraFlags).toBe("--model haiku");
  });

  // T-UNIT-TS-034
  it("AliasDetector_should_returnAlias_When_fullGrammarAllParts", () => {
    const result = detector.detect("@myproj:feat/auth working on auth --model haiku");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.aliasName).toBe("myproj");
    expect(result?.metadata?.branch).toBe("feat/auth");
    expect(result?.metadata?.label).toBe("working on auth");
    expect(result?.metadata?.extraFlags).toBe("--model haiku");
  });

  // T-UNIT-TS-035
  it("AliasDetector_should_returnAliasNotFound_When_unknownSlugWithSpace", () => {
    const result = detector.detect("@nope ");
    expect(result?.type).toBe(InputType.AliasNotFound);
    expect(result?.metadata?.slug).toBe("nope");
  });

  // T-UNIT-TS-038
  it("AliasDetector_should_matchCaseInsensitive_When_upperCaseAliasName", () => {
    const result = detector.detect("@MYPROJ ");
    expect(result?.type).toBe(InputType.Alias);
  });

  // T-UNIT-TS-040
  it("AliasDetector_should_neverReturnNull_When_inputStartsWithAt", () => {
    const inputs = ["@", "@a", "@unknown ", "@MYPROJ ", "@myproj:branch label --flag"];
    for (const input of inputs) {
      const result = detector.detect(input);
      expect(result).not.toBeNull();
    }
  });

  it("AliasDetector_should_returnNull_When_inputDoesNotStartWithAt", () => {
    expect(detector.detect("myproj")).toBeNull();
    expect(detector.detect("/path/to/thing")).toBeNull();
    expect(detector.detect("")).toBeNull();
  });

  it("AliasDetector_should_parseEmptyBranchAndLabel_When_onlyNameWithSpace", () => {
    const result = detector.detect("@myproj ");
    expect(result?.type).toBe(InputType.Alias);
    expect(result?.metadata?.branch).toBeUndefined();
    expect(result?.metadata?.label).toBeUndefined();
    expect(result?.metadata?.extraFlags).toBeUndefined();
  });
});

describe("AliasDetector (empty alias list)", () => {
  let emptyDetector: AliasDetector;

  beforeEach(() => {
    emptyDetector = new AliasDetector([]);
  });

  it("AliasDetector_should_returnAliasBrowse_When_bareAtSignAndNoAliases", () => {
    const result = emptyDetector.detect("@");
    expect(result?.type).toBe(InputType.AliasBrowse);
  });

  it("AliasDetector_should_returnAliasNotFound_When_nameWithSpaceAndNoAliases", () => {
    const result = emptyDetector.detect("@anything ");
    expect(result?.type).toBe(InputType.AliasNotFound);
  });

  it("AliasDetector_should_returnNull_When_noAtPrefixAndNoAliases", () => {
    expect(emptyDetector.detect("myproj")).toBeNull();
  });
});
