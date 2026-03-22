# Stapler Squad E2E Tests

End-to-end tests for stapler-squad web UI with **isolated test mode** to prevent interference with production data.

## Features

✅ **Isolated Test Environment**: Tests run with dedicated data directory (`/tmp/stapler-squad-test-<PID>`)
✅ **Automatic Server Management**: Test server starts/stops automatically with isolated state
✅ **No Production Data Contamination**: All test data is ephemeral and cleaned up after tests
✅ **Separate Port**: Test server runs on port 8544 (production uses 8543)
✅ **Automatic Cleanup**: Test directories are removed after test completion

## Quick Start

```bash
# Install dependencies
cd tests/e2e
npm install

# Install Playwright browsers (first time only)
npx playwright install chromium

# Run tests (headless) - server starts automatically in test mode
npm test

# Run tests with browser UI
npm run test:headed

# Run tests in debug mode
npm run test:debug

# Run tests with interactive UI
npm run test:ui

# Run specific test file
npx playwright test smoke.spec.ts
```

## How It Works

### Test Mode Architecture

1. **Global Setup** (`global-setup.ts`):
   - Builds Go binary if needed
   - Starts server with `--test-mode --test-dir /tmp/stapler-squad-test-<PID>`
   - Waits for server health check
   - Server runs on port 8544 with isolated data

2. **Test Execution**:
   - All tests use `http://localhost:8544` (configured in `playwright.config.ts`)
   - Test data stored in `/tmp/stapler-squad-test-<PID>`
   - No interference with production data at `~/.stapler-squad`

3. **Global Teardown** (`global-teardown.ts`):
   - Stops test server gracefully
   - Removes test data directory
   - Cleans up all test resources

### Manual Test Mode

You can also run the server manually in test mode:

```bash
# From project root
./stapler-squad --web --test-mode --test-dir /tmp/my-test-data

# Or with custom test directory
./stapler-squad --web --test-mode --test-dir ~/tmp/custom-test-dir
```

## Test Coverage

### Smoke Tests ✅
- Review queue page loads successfully
- Home page loads successfully
- Navigation header is present

### Real-Time Updates
- Queue updates within 100ms on terminal input
- Badge reflects real-time queue count changes
- WebSocket push updates

### Keyboard Navigation
- Forward navigation with `]` key
- Backward navigation with `[` key
- Circular navigation (wraps at end)
- Shortcuts disabled in input fields

### Optimistic UI
- Immediate item removal on acknowledgment
- Rollback on API failure
- Multiple rapid acknowledgments

### WebSocket Events
- Initial snapshot loads immediately
- Multi-client updates synchronize
- Statistics updates

### Performance
- <100ms latency under load
- Concurrent operations handling

## Test Structure

```
tests/e2e/
├── helpers/
│   └── test-server.ts         # Test server management
├── global-setup.ts            # Start test server
├── global-teardown.ts         # Stop and cleanup
├── smoke.spec.ts              # Basic smoke tests
├── review-queue.spec.ts       # Review queue tests
├── playwright.config.ts       # Playwright configuration
├── package.json              # Test dependencies
└── README.md                 # This file
```

## Writing New Tests

1. Follow the existing test patterns
2. Use `data-testid` attributes for element selection
3. Add helper functions for common operations
4. Document expected behavior clearly

Example:
```typescript
import { test, expect } from '@playwright/test';

const BASE_URL = 'http://localhost:8544'; // Test server URL

test('my new test', async ({ page }) => {
  await page.goto(`${BASE_URL}/my-page`);

  // Your test logic
  await page.click('[data-testid="my-button"]');

  // Assertions
  await expect(page.locator('[data-testid="result"]'))
    .toBeVisible({ timeout: 5000 });
});
```

## Required `data-testid` Attributes

The tests expect the following attributes in the web UI:

### Review Queue Page
- `review-queue` - Main queue container
- `review-queue-badge` - Badge showing item count (in header)
- `review-item` - Individual queue items
- `review-item-${sessionId}` - Specific item by session ID
- `current-item` - Currently selected/navigated item
- `acknowledge-${sessionId}` - Acknowledge button for session
- `queue-statistics` - Statistics panel
- `total-items` - Total items count
- `review-queue-loaded` - Hidden indicator that queue has loaded

### Session Creation Page
- `session-title` - Title input field
- `session-path` - Path input field
- `auto-yes-checkbox` - Auto-yes checkbox
- `create-session-button` - Submit button
- `session-created` - Success indicator

### Terminal
- `terminal` - Terminal container for input
- `open-terminal` - Button to open terminal

## Debugging Tests

### Visual Debugging
```bash
# See browser during test execution
npm run test:headed

# Step through tests interactively
npm run test:debug

# Use Playwright UI mode
npm run test:ui
```

### Screenshots and Videos
Failed tests automatically capture:
- Screenshots (on-failure)
- Videos (on-failure)
- Browser traces (on-retry)

Find them in: `test-results/`

### Server Logs
The test server logs are available:
```bash
# Watch test server output during development
PLAYWRIGHT_DEBUG=1 npm test
```

### Console Logs
Tests output performance metrics:
```
Queue update latency: 47ms
Optimistic remove latency: 12ms
Average queue update latency: 63ms
```

## Performance Targets

- ✅ Queue updates: <100ms
- ✅ Optimistic UI: <50ms
- ✅ Initial load: <500ms
- ✅ WebSocket connection: <1s

## Troubleshooting

### "Failed to start test server"
- Check that Go is installed: `go version`
- Ensure project builds: `cd ../.. && go build .`
- Check port 8544 is available: `lsof -i :8544`

### "Server not responding"
- Test server should start automatically
- Check if port 8544 is available
- Look for build errors in test output

### "Element not found"
- Check that data-testid attributes are implemented in the web UI
- Verify you're testing against the correct base URL (8544 for tests)

### "Timeout waiting for event"
- Increase timeout in test: `{ timeout: 10000 }`
- Check server logs for issues
- Verify WebSocket connections are working

### "Tests fail intermittently"
- Increase timeouts in playwright.config.ts
- Check for race conditions
- Ensure test isolation (use unique session names per test)
- Verify test data is properly cleaned up between runs

### "Test data directory not cleaned up"
Test directories should be automatically removed. If not:
```bash
# Manual cleanup
rm -rf /tmp/stapler-squad-test-*
```

## CI/CD Integration

The tests are designed to run in CI environments with automatic test server management:

```yaml
# .github/workflows/e2e-tests.yml
name: E2E Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: 18
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install test dependencies
        run: cd tests/e2e && npm install

      - name: Install Playwright browsers
        run: cd tests/e2e && npx playwright install --with-deps chromium

      - name: Run E2E tests
        run: cd tests/e2e && npm test

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: playwright-report
          path: tests/e2e/test-results/
```

## Test Mode Implementation Details

### CLI Flags

```bash
./stapler-squad --help
Flags:
  --test-mode              Run in test mode with isolated data directory
  --test-dir string        Custom test data directory (defaults to /tmp/stapler-squad-test-<PID>)
  --web                    Run HTTP server with ConnectRPC API
```

### Environment Variables

The `--test-mode` flag sets `STAPLER_SQUAD_TEST_DIR` internally. You can also set it manually:

```bash
# Manual test mode (without --test-mode flag)
export STAPLER_SQUAD_TEST_DIR=/tmp/my-test-data
./stapler-squad --web
```

### Data Isolation Hierarchy

Config directory resolution priority:
1. **Test directory** (`STAPLER_SQUAD_TEST_DIR`) - Highest priority
2. Explicit instance ID (`STAPLER_SQUAD_INSTANCE`)
3. Auto-detected test mode (go test)
4. Workspace-based isolation (default)
5. Global shared state (fallback)

This ensures test data never interferes with production data.

## Additional Resources

- [Playwright Documentation](https://playwright.dev/)
- [Best Practices](https://playwright.dev/docs/best-practices)
- [Debugging Guide](https://playwright.dev/docs/debug)
- [Stapler Squad Test Mode](../../CLAUDE.md#test-mode)
