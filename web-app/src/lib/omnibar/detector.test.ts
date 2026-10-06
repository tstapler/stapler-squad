/**
 * Tests for the Omnibar Detector / InputType registry.
 *
 * Covers:
 *  T-UNIT-TS-008: Bare word resolves to SessionSearch
 *  T-UNIT-TS-009: Empty string resolves to Unknown (not SessionSearch)
 *  T-UNIT-TS-010: Path input resolves to LocalPath (not displaced by SessionSearch)
 *  T-UNIT-TS-011: GitHub shorthand resolves to GitHubShorthand (not displaced by SessionSearch)
 *  T-PITFALL-001: Bare text does not silently fall through to Unknown
 *  T-PITFALL-002: Hyphenated bare text resolves to SessionSearch
 */

import { InputType, DetectionResult } from "@/lib/omnibar/types";
import { createDefaultRegistry, DetectorRegistry, Detector } from "@/lib/omnibar/detector";

describe("Detector", () => {
  // Use a fresh registry per test-suite to avoid singleton state leakage
  let registry: ReturnType<typeof createDefaultRegistry>;

  beforeEach(() => {
    registry = createDefaultRegistry();
  });

  // T-UNIT-TS-008
  describe("SessionSearchDetector", () => {
    it("resolves bare word to SessionSearch", () => {
      const result = registry.detect("squad");
      expect(result.type).toBe(InputType.SessionSearch);
      expect(result.parsedValue).toBe("squad");
    });

    // T-UNIT-TS-009
    it("returns Unknown for empty string (not SessionSearch)", () => {
      const result = registry.detect("");
      expect(result.type).not.toBe(InputType.SessionSearch);
      expect(result.type).toBe(InputType.Unknown);
    });
  });

  // T-UNIT-TS-010
  it("path input resolves to LocalPath (not displaced by SessionSearch)", () => {
    const result = registry.detect("~/projects");
    expect(result.type).toBe(InputType.LocalPath);
  });

  // T-UNIT-TS-011
  it("GitHub shorthand resolves to GitHubShorthand (not displaced by SessionSearch)", () => {
    const result = registry.detect("org/repo");
    expect(result.type).toBe(InputType.GitHubShorthand);
  });

  describe("NewSessionDetector", () => {
    it("NewSessionDetector_should_detectNewPrefix_When_inputStartsWithNew", () => {
      const result = registry.detect("new/");
      expect(result.type).toBe(InputType.NewSession);
    });

    it("NewSessionDetector_should_returnNull_When_inputDoesNotStartWithNew", () => {
      const result = registry.detect("stapler");
      expect(result.type).not.toBe(InputType.NewSession);
    });

    it("NewSessionDetector_should_parseQueryAfterPrefix_When_inputIsNewSlashFoo", () => {
      const result = registry.detect("new/stapler");
      expect(result.type).toBe(InputType.NewSession);
      expect(result.parsedValue).toBe("stapler");
    });

    it("NewSessionDetector_should_returnEmptyParsedValue_When_inputIsJustNewSlash", () => {
      const result = registry.detect("new/");
      expect(result.type).toBe(InputType.NewSession);
      expect(result.parsedValue).toBe("");
    });

    it("NewSessionDetector_should_detectPrefix_When_inputIsUppercaseNEW", () => {
      const result = registry.detect("NEW/thing");
      expect(result.type).toBe(InputType.NewSession);
      expect(result.parsedValue).toBe("thing");
    });
  });

  describe("ChatBacklogItemDetector", () => {
    it("ChatBacklogItemDetector_should_detectBacklogPrefix_When_inputStartsWithBacklogColon", () => {
      const result = registry.detect("backlog: add dark mode support");
      expect(result.type).toBe(InputType.ChatBacklogItem);
      expect(result.parsedValue).toBe("add dark mode support");
    });

    it("ChatBacklogItemDetector_should_beCaseInsensitive_When_prefixIsUppercase", () => {
      const result = registry.detect("BACKLOG: fix the thing");
      expect(result.type).toBe(InputType.ChatBacklogItem);
      expect(result.parsedValue).toBe("fix the thing");
    });

    it("ChatBacklogItemDetector_should_returnNull_When_inputHasNoBacklogPrefix", () => {
      const result = registry.detect("just some plain text");
      expect(result.type).not.toBe(InputType.ChatBacklogItem);
    });

    it("ChatBacklogItemDetector_should_returnNull_When_messageAfterPrefixIsEmpty", () => {
      const result = registry.detect("backlog:   ");
      expect(result.type).not.toBe(InputType.ChatBacklogItem);
    });

    it("ChatBacklogItemDetector_should_notShadowGitHubPRDetector_When_inputIsAGitHubPRUrl", () => {
      const result = registry.detect("https://github.com/owner/repo/pull/123");
      expect(result.type).toBe(InputType.GitHubPR);
    });

    it("ChatBacklogItemDetector_should_notBeShadowedByNewSessionDetector_When_inputHasBacklogPrefix", () => {
      const result = registry.detect("backlog: new session idea");
      expect(result.type).toBe(InputType.ChatBacklogItem);
    });
  });

  // T-PITFALL-001
  describe("pitfall guards", () => {
    it("bare text does not resolve to Unknown (T-PITFALL-001)", () => {
      const result = registry.detect("squad");
      expect(result.type).not.toBe(InputType.Unknown);
      expect(result.type).toBe(InputType.SessionSearch);
    });

    // T-PITFALL-002
    it("hyphenated bare text resolves to SessionSearch (T-PITFALL-002)", () => {
      const result = registry.detect("my-feature");
      expect(result.type).toBe(InputType.SessionSearch);
    });
  });

  describe("default registry @-prefix fallthrough", () => {
    it("should_returnSessionSearch_When_atPrefixedInputAndNoWorkflowDetectorRegistered", () => {
      // WorkflowDetector is NOT in the default registry; @-prefixed input should
      // fall through to SessionSearch (the catch-all), never resolve to Workflow.
      const result = registry.detect("@daily-standup");
      expect(result.type).not.toBe(InputType.Workflow);
      expect(result.type).toBe(InputType.SessionSearch);
    });
  });

  // AC3: an unhandled exception thrown by one detector must not silently abandon
  // detection — lower-priority detectors (and the next debounce tick) must still run.
  describe("detector exception handling (AC3)", () => {
    class ThrowingDetector implements Detector {
      name = "Throwing";
      priority = 1; // runs before every default detector
      detect(): DetectionResult | null {
        throw new Error("boom");
      }
    }

    let consoleErrorSpy: jest.SpyInstance;

    beforeEach(() => {
      consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    });

    afterEach(() => {
      consoleErrorSpy.mockRestore();
    });

    it("detect() falls through to a lower-priority detector when a higher-priority one throws", () => {
      registry.register(new ThrowingDetector());
      const result = registry.detect("squad");
      expect(result.type).toBe(InputType.SessionSearch);
      expect(consoleErrorSpy).toHaveBeenCalled();
    });

    it("detect() still returns Unknown (not a crash) when every registered detector throws", () => {
      const isolated = new DetectorRegistry();
      isolated.register(new ThrowingDetector());
      const result = isolated.detect("anything");
      expect(result.type).toBe(InputType.Unknown);
    });

    it("a throw on one debounce tick does not break detection on the next", () => {
      const flaky = new ThrowingDetector();
      registry.register(flaky);
      expect(registry.detect("squad").type).toBe(InputType.SessionSearch);

      // Simulate the detector recovering (e.g. transient error) on a later tick —
      // the registry itself carries no broken state from the earlier throw.
      registry.unregister(flaky);
      expect(registry.detect("squad").type).toBe(InputType.SessionSearch);
    });
  });

  // detectAll() mirrors detect()'s try/catch around each detector.detect() call
  // but was added without its own test coverage — verify the same resilience
  // guarantees hold: a throwing detector must not prevent detectAll() from
  // returning results collected from the other, non-throwing detectors.
  describe("detectAll() exception handling", () => {
    class ThrowingDetector implements Detector {
      name = "Throwing";
      priority = 1; // runs before every default detector
      detect(): DetectionResult | null {
        throw new Error("boom");
      }
    }

    let consoleErrorSpy: jest.SpyInstance;

    beforeEach(() => {
      consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    });

    afterEach(() => {
      consoleErrorSpy.mockRestore();
    });

    it("does not throw and still returns results from other detectors when one detector throws", () => {
      registry.register(new ThrowingDetector());

      let results: DetectionResult[] = [];
      expect(() => {
        results = registry.detectAll("squad");
      }).not.toThrow();

      expect(results.some((r) => r.type === InputType.SessionSearch)).toBe(true);
      expect(consoleErrorSpy).toHaveBeenCalled();
    });

    it("returns an empty array (not a crash) when every registered detector throws", () => {
      const isolated = new DetectorRegistry();
      isolated.register(new ThrowingDetector());

      let results: DetectionResult[] = [];
      expect(() => {
        results = isolated.detectAll("anything");
      }).not.toThrow();

      expect(results).toEqual([]);
      expect(consoleErrorSpy).toHaveBeenCalled();
    });
  });

  describe("GitHubEnterpriseURL registration", () => {
    // Regression test: GitHubEnterpriseURLDetector was previously only
    // registered from an async useEffect in OmnibarContext, keyed on the
    // GHES host RPC result. That left a window on every fresh page load
    // where a GHES PR/branch/repo URL had no matching detector and fell
    // through to SessionSearchDetector's catch-all, producing a garbled
    // slugified "session name" instead of being recognized. The detector
    // must now be present in the registry synchronously, before any host
    // list has loaded.
    it("is present synchronously in createDefaultRegistry(), before any host list is set", () => {
      expect(registry.find("GitHubEnterpriseURL")).toBeDefined();
    });

    it("finds the same registered instance across find() calls (singleton, not a copy)", () => {
      const first = registry.find("GitHubEnterpriseURL");
      const second = registry.find("GitHubEnterpriseURL");
      expect(first).toBe(second);
    });
  });
});
