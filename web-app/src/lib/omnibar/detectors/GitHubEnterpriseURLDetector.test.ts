import { GitHubEnterpriseURLDetector } from "./GitHubEnterpriseURLDetector";
import { InputType } from "../types";

describe("GitHubEnterpriseURLDetector", () => {
  const detector = new GitHubEnterpriseURLDetector(["github.example.com"]);

  it("T-UNIT-TS-201 GitHubEnterpriseURLDetector_should_detectPR_When_configuredHostPullUrl", () => {
    const result = detector.detect("https://github.example.com/acme/widgets/pull/42");
    expect(result?.type).toBe(InputType.GitHubPR);
    expect(result?.gitHubRef).toEqual({ owner: "acme", repo: "widgets", prNumber: 42 });
    expect(result?.suggestedName).toBe("acme-widgets-pr-42");
  });

  it("T-UNIT-TS-202 GitHubEnterpriseURLDetector_should_detectBranch_When_configuredHostTreeUrl", () => {
    const result = detector.detect("https://github.example.com/acme/widgets/tree/feature/foo");
    expect(result?.type).toBe(InputType.GitHubBranch);
    expect(result?.gitHubRef).toEqual({
      owner: "acme",
      repo: "widgets",
      branch: "feature/foo",
    });
    expect(result?.suggestedName).toBe("acme-widgets-feature-foo");
  });

  it("T-UNIT-TS-203 GitHubEnterpriseURLDetector_should_detectRepo_When_configuredHostBareRepoUrl", () => {
    const result = detector.detect("https://github.example.com/acme/widgets");
    expect(result?.type).toBe(InputType.GitHubRepo);
    expect(result?.gitHubRef).toEqual({ owner: "acme", repo: "widgets" });
    expect(result?.suggestedName).toBe("widgets");
  });

  it("T-UNIT-TS-204 GitHubEnterpriseURLDetector_should_stripGitSuffix_When_bareRepoUrlHasDotGit", () => {
    const result = detector.detect("https://github.example.com/acme/widgets.git");
    expect(result?.gitHubRef).toEqual({ owner: "acme", repo: "widgets" });
  });

  it("T-PITFALL-201 GitHubEnterpriseURLDetector_should_returnNull_When_hostNotConfigured", () => {
    expect(detector.detect("https://github.other.com/acme/widgets/pull/1")).toBeNull();
  });

  it("T-PITFALL-202 GitHubEnterpriseURLDetector_should_returnNull_When_githubComUrl", () => {
    expect(detector.detect("https://github.com/acme/widgets/pull/1")).toBeNull();
  });

  it("T-PITFALL-203 GitHubEnterpriseURLDetector_should_returnNull_When_noHostsConfigured", () => {
    const empty = new GitHubEnterpriseURLDetector([]);
    expect(empty.detect("https://github.example.com/acme/widgets")).toBeNull();
  });

  it("T-PITFALL-204 GitHubEnterpriseURLDetector_should_treatHostAsLiteral_When_hostContainsRegexChars", () => {
    const dotted = new GitHubEnterpriseURLDetector(["github.example.com"]);
    // "githubXexampleXcom" would match if the "." were interpreted as regex wildcard.
    expect(dotted.detect("https://githubXexampleXcom/acme/widgets")).toBeNull();
  });

  it("T-UNIT-TS-205 GitHubEnterpriseURLDetector_should_detectConfiguredHost_When_setHostsCalledAfterConstruction", () => {
    // Regression test: the detector starts with an empty host list at
    // createDefaultRegistry() time (before the GHES host RPC resolves), then
    // OmnibarContext calls setHosts() once the real host list loads. A URL for
    // a host must be undetectable before setHosts() and detectable after.
    const lazy = new GitHubEnterpriseURLDetector();
    expect(lazy.detect("https://github.example.com/acme/widgets/pull/42")).toBeNull();

    lazy.setHosts(["github.example.com"]);
    const result = lazy.detect("https://github.example.com/acme/widgets/pull/42");
    expect(result?.type).toBe(InputType.GitHubPR);
    expect(result?.gitHubRef).toEqual({ owner: "acme", repo: "widgets", prNumber: 42 });
  });

  it("T-UNIT-TS-206 GitHubEnterpriseURLDetector_should_stopDetectingOldHost_When_setHostsReplacesList", () => {
    const detector = new GitHubEnterpriseURLDetector(["github.example.com"]);
    detector.setHosts(["github.other-example.com"]);

    expect(detector.detect("https://github.example.com/acme/widgets/pull/1")).toBeNull();
    expect(detector.detect("https://github.other-example.com/acme/widgets/pull/1")?.type).toBe(
      InputType.GitHubPR
    );
  });
});
