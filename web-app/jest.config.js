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
        // vanilla-extract .css.ts files — return callable proxy
        "\\.css\\.ts$": "<rootDir>/src/__mocks__/styleMock.js",
        "\\.module\\.css$": "identity-obj-proxy",
        "\\.css$": "<rootDir>/src/__mocks__/styleMock.js",
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
      // pnpm's ".pnpm/<pkg>@<version>/node_modules/<pkg>" store layout, just
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
