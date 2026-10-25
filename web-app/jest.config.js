/** @type {import('jest').Config} */
module.exports = {
  projects: [
    {
      displayName: "web-app",
      preset: "ts-jest",
      testEnvironment: "jest-environment-jsdom",
      roots: ["<rootDir>/src"],
      // Narrowed to *.test.ts(x)/*.spec.ts(x) (2026-08-24): the previous
      // "**/__tests__/**/*.ts?(x)" pattern matched any file inside a
      // __tests__/ directory, not just test files. A repo-wide search found
      // zero existing test files that relied on the broad form — every real
      // suite already matches the narrower pattern below — but 7 non-test
      // fixture/mock helper files (extracted while deduplicating jscpd
      // findings) newly hit it, each failing CI with "Your test suite must
      // contain at least one test". Narrowing here fixes the root cause so a
      // future __tests__/ helper file can't silently break the same way.
      testMatch: ["**/?(*.)+(spec|test).ts?(x)"],
      // Default 5000ms is too tight for this project's full-suite runs: with
      // ~350 suites sharing jest-worker processes, scheduler contention alone
      // has intermittently pushed otherwise-fast async tests (e.g.
      // BacklogItemDetail.loadGuard, SessionDetailView.note-error) over 5s
      // with no logic bug — each passes in well under 1s run in isolation.
      testTimeout: 20000,
      moduleNameMapper: {
        // Window style modules get string class names (the proxy below trips React's
        // className check); these must stay ahead of the generic .css mappers.
        "styles/window/windowTabStrip\\.css(\\.ts)?$": "<rootDir>/src/__mocks__/windowTabStripStylesMock.js",
        "styles/window/windowOverview\\.css(\\.ts)?$": "<rootDir>/src/__mocks__/windowOverviewStylesMock.js",
        // vanilla-extract .css.ts files — return callable proxy
        "\\.css\\.ts$": "<rootDir>/src/__mocks__/styleMock.js",
        "\\.module\\.css$": "identity-obj-proxy",
        "\\.css$": "<rootDir>/src/__mocks__/styleMock.js",
        // GuidanceRequestPanel (AC3, embedded in BacklogItemDetail/TriageReviewPanel/
        // SessionDetailView) uses RTK Query hooks needing a real Redux <Provider> most
        // suites don't set up — map globally instead of per-suite jest.mock calls. A
        // suite wanting realistic data can still override with its own jest.mock.
        "^@/lib/api/guidanceApi$": "<rootDir>/src/__mocks__/guidanceApiMock.js",
        // Lazy-loading seam: jest uses the Next impl (suites mock next/dynamic); stale-chunk tests import impl.bun directly.
        "^@/lib/lazy/impl$": "<rootDir>/src/lib/lazy/impl.next.tsx",
        // Navigation seam: only reached by suites that load the real facade; most mock "@/lib/nav" (jest.setup.js).
        "^@/lib/nav/impl$": "<rootDir>/src/lib/nav/impl.next.tsx",
        // Path alias
        "^@/(.*)$": "<rootDir>/src/$1",
      },
      setupFilesAfterEnv: ["<rootDir>/jest.setup.js"],
      transform: {
        "^.+\\.[tj]sx?$": [
          "ts-jest",
          {
            tsconfig: {
              jsx: "react-jsx",
              module: "commonjs",
              moduleResolution: "node",
              esModuleInterop: true,
            },
          },
        ],
      },
      // react-markdown pulls in a deep tree of ESM-only unified/remark/rehype/
      // mdast/hast transitive deps (new one surfaces every level down). Rather
      // than hand-maintain an ever-growing allowlist of name prefixes against
      // the isolated linker's "node_modules/.bun/<pkg>@<version>/node_modules/<pkg>" store layout, just
      // transform all of node_modules — ts-jest handles CJS and ESM alike.
      transformIgnorePatterns: [],
    },
    {
      displayName: "eslint-plugin-analytics",
      testEnvironment: "node",
      roots: ["<rootDir>/eslint-plugin-analytics"],
      testMatch: ["**/__tests__/**/*.js", "**/?(*.)+(spec|test).js"],
      transform: {},
    },
    {
      displayName: "dev-stack",
      preset: "ts-jest",
      testEnvironment: "node",
      roots: ["<rootDir>/../scripts/dev-stack"],
      testMatch: ["**/?(*.)+(spec|test).ts"],
      transform: {
        "^.+\\.tsx?$": [
          "ts-jest",
          {
            tsconfig: {
              module: "commonjs",
              moduleResolution: "node",
              esModuleInterop: true,
            },
          },
        ],
      },
    },
    {
      // Node-side tests of the Bun build. Bun-only behaviour runs in a pinned `bun`
      // subprocess via runInBun; `*.bun.ts` scripts are not tests and are never collected.
      // scripts/__tests__/*.test.mjs use node:test and run through `bun run test:codemods`.
      displayName: "build",
      preset: "ts-jest",
      testEnvironment: "node",
      roots: ["<rootDir>/build"],
      testMatch: ["<rootDir>/build/**/__tests__/**/*.test.ts"],
      testPathIgnorePatterns: ["/node_modules/", "/fixtures/"],
      transform: {
        "^.+\\.tsx?$": [
          "ts-jest",
          {
            tsconfig: {
              module: "commonjs",
              moduleResolution: "node",
              esModuleInterop: true,
              types: ["node", "jest"],
            },
          },
        ],
      },
    },
    {
      // tests/e2e/ hosts the Playwright suite (many *.spec.ts files that
      // import '@playwright/test' and are NOT runnable under Jest), plus a
      // small number of plain Jest unit tests for the dev-mode harness
      // itself (Epic 5.1, e.g. global-setup.dev-mode.test.ts). testMatch is
      // deliberately restricted to "*.test.ts" only (never "*.spec.ts") so
      // this project never picks up any Playwright spec file.
      displayName: "e2e-dev-mode",
      preset: "ts-jest",
      testEnvironment: "node",
      roots: ["<rootDir>/../tests/e2e"],
      testMatch: ["**/?(*.)+(test).ts"],
      transform: {
        "^.+\\.tsx?$": [
          "ts-jest",
          {
            tsconfig: {
              module: "commonjs",
              moduleResolution: "node",
              esModuleInterop: true,
            },
          },
        ],
      },
    },
  ],
};
