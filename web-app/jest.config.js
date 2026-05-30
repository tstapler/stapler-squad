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
  ],
};
