/** @type {import('jest').Config} */
module.exports = {
  projects: [
    {
      displayName: "web-app",
      preset: "ts-jest",
      testEnvironment: "jest-environment-jsdom",
      roots: ["<rootDir>/src"],
      testMatch: ["**/__tests__/**/*.ts?(x)", "**/?(*.)+(spec|test).ts?(x)"],
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
      transformIgnorePatterns: [
        "/node_modules/(?!(@bufbuild|@connectrpc)/)",
      ],
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
