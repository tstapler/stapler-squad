// @feature session:shell-tabs
import { test, expect } from '@playwright/test';
import { ShellTabsPage } from './pages/ShellTabsPage';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// ---------------------------------------------------------------------------
// Helper: navigate to the app, wait for a session to be visible, then open it.
// All shell-tab interactions require a session detail view to be visible.
// ---------------------------------------------------------------------------
async function openSessionDetailView(page: import('@playwright/test').Page): Promise<ShellTabsPage> {
  const shellTabs = new ShellTabsPage(page);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  // Wait for at least one session card or session row
  await page.waitForSelector(
    '[data-testid="session-card"], [data-testid="session-row"]',
    { timeout: 15000 },
  );
  await shellTabs.openFirstSession();
  return shellTabs;
}

// ---------------------------------------------------------------------------
// Fixture: ensure a session exists before each test via the SessionClient RPC.
// The test server is isolated so the backlog may be empty on a fresh run.
// ---------------------------------------------------------------------------
test.beforeAll(async () => {
  const client = new SessionClient(BASE_URL);
  const sessions = await client.listSessions();
  if (sessions.length === 0) {
    // Create a minimal directory session using /tmp (always present) so the UI
    // has something to display.  Program is "bash" — no AI process is started.
    await client.createSession({
      title: 'shell-tabs-e2e-fixture',
      path: '/tmp',
      program: 'bash',
    });
  }
});

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

test.describe('shell-tabs', () => {
  // -------------------------------------------------------------------------
  // TC-1: spawn_new_shell_via_button
  // User clicks the "+" button in the shell tab bar, fills the dialog,
  // and a new shell tab appears in the tab strip.
  // -------------------------------------------------------------------------
  test(
    'shell-tabs_should_addNewShellTab_When_plusButtonClicked',
    async ({ page }) => {
      const shellTabs = await openSessionDetailView(page);

      // Count shell tabs before spawning
      const tabsBefore = await page.getByRole('tab').count();

      // Open dialog via "+" button and fill optional name
      await shellTabs.openDialogViaButton({ name: 'e2e-bash' });

      // Verify dialog is open and the name field has been filled correctly
      await expect(shellTabs.shellNameInput).toHaveValue('e2e-bash');

      // Intercept the SpawnShell RPC to avoid waiting for a real PTY
      const rpcPromise = page.waitForRequest(
        (req) => req.url().includes('SpawnShell') && req.method() === 'POST',
        { timeout: 10000 },
      ).catch(() => null); // non-blocking — dialog close is the primary assertion

      await shellTabs.submitAndWait();

      // A new shell tab must appear in the tab bar (tab count should have grown)
      await expect(async () => {
        const tabsAfter = await page.getByRole('tab').count();
        expect(tabsAfter).toBeGreaterThan(tabsBefore);
      }).toPass({ timeout: 8000 });

      // The new tab must display the name we supplied
      await expect(shellTabs.getShellTab('e2e-bash')).toBeVisible({ timeout: 8000 });

      await rpcPromise; // resolve silently if the request was captured
    },
  );

  // -------------------------------------------------------------------------
  // TC-2: spawn_new_shell_via_keyboard_shortcut
  // User presses Ctrl+T while the terminal area is focused; the dialog opens.
  // Filling the dialog and submitting adds a new shell tab.
  // -------------------------------------------------------------------------
  test(
    'shell-tabs_should_openNewShellDialog_When_ctrlTPressed',
    async ({ page }) => {
      const shellTabs = await openSessionDetailView(page);

      // Switch to the Terminal tab so the shortcut context is "terminal"
      const terminalTab = page.getByRole('tab', { name: /terminal/i });
      await expect(terminalTab).toBeVisible({ timeout: 5000 });
      await terminalTab.click();

      // Give the terminal area focus so useShortcut fires in the "terminal" context
      await terminalTab.press('Tab'); // move focus into the terminal panel area

      // Press Ctrl+T to invoke the shortcut
      await page.keyboard.press('Control+t');

      // The New Shell dialog must open
      await shellTabs.waitForDialog();
      await expect(shellTabs.newShellDialog).toBeVisible();

      // Fill a distinctive name so we can assert the tab appeared
      await shellTabs.shellNameInput.fill('e2e-shortcut-shell');
      await expect(shellTabs.shellNameInput).toHaveValue('e2e-shortcut-shell');

      const tabsBefore = await page.getByRole('tab').count();

      await shellTabs.submitAndWait();

      // A new tab with the supplied name must appear
      await expect(async () => {
        const tabsAfter = await page.getByRole('tab').count();
        expect(tabsAfter).toBeGreaterThan(tabsBefore);
      }).toPass({ timeout: 8000 });

      await expect(shellTabs.getShellTab('e2e-shortcut-shell')).toBeVisible({ timeout: 8000 });
    },
  );

  // -------------------------------------------------------------------------
  // TC-3: close_shell_tab
  // User spawns a shell, then clicks the "×" delete button on the tab; the tab
  // disappears from the tab strip.
  // -------------------------------------------------------------------------
  test(
    'shell-tabs_should_removeTab_When_deleteButtonClicked',
    async ({ page }) => {
      const shellTabs = await openSessionDetailView(page);

      // Spawn a shell to close
      await shellTabs.openDialogViaButton({ name: 'e2e-close-me' });
      await shellTabs.submitAndWait();

      // Verify the new tab is present
      const shellTab = shellTabs.getShellTab('e2e-close-me');
      await expect(shellTab).toBeVisible({ timeout: 8000 });

      // Click the delete (×) button within the tab label.
      // aria-label="Delete shell e2e-close-me" is set in ShellTab.tsx.
      const deleteButton = shellTabs.getDeleteShellButton('e2e-close-me');
      await expect(deleteButton).toBeVisible({ timeout: 5000 });
      await deleteButton.click();

      // The tab must disappear
      await expect(shellTab).not.toBeVisible({ timeout: 8000 });
    },
  );

  // -------------------------------------------------------------------------
  // TC-4: spawn_new_shell_via_action_menu
  // User clicks the "⋯" / "Session actions" button in the session header,
  // selects "Spawn new shell", and the New Shell dialog opens.
  // -------------------------------------------------------------------------
  test(
    'shell-tabs_should_openNewShellDialog_When_actionMenuSpawnShellClicked',
    async ({ page }) => {
      const shellTabs = await openSessionDetailView(page);

      // Open the action menu
      await shellTabs.moreActionsButton.click();

      // The action sheet must open with the "Spawn new shell" option visible
      await expect(shellTabs.actionSheetSpawnShell).toBeVisible({ timeout: 5000 });

      // Click "Spawn new shell" inside the action sheet
      await shellTabs.actionSheetSpawnShell.click();

      // The New Shell dialog must open
      await shellTabs.waitForDialog();
      await expect(shellTabs.newShellDialog).toBeVisible();

      // Name and command fields must be present and editable
      await expect(shellTabs.shellNameInput).toBeVisible();
      await expect(shellTabs.shellCommandInput).toBeVisible();

      // Close without submitting — press Escape
      await page.keyboard.press('Escape');
      await shellTabs.waitForDialogClosed();
    },
  );
});
