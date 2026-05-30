// jest.setup.js — runs after the test framework is installed in the environment.
//
// Polyfills TextEncoder/TextDecoder which are missing from older jsdom versions
// but required by @bufbuild/protobuf.
const { TextEncoder, TextDecoder } = require("util");
Object.assign(globalThis, { TextEncoder, TextDecoder });

// jest-dom matchers (toBeInTheDocument, etc.)
require("@testing-library/jest-dom");
