import { test, expect } from '@playwright/test';
import { launchSession, navigateToStressTest } from './helpers';

/**
 * Test to reproduce terminal flickering with rapid color changes.
 * This simulates the "color wave" effect seen when Claude outputs
 * rapidly changing ANSI color codes.
 */
test.describe('Rapid Color Flickering', () => {
  test('should handle rapid color changes without flickering', async ({ page }) => {
    await navigateToStressTest(page);
    const sessionName = await launchSession(page, 'color-flicker');

    // Generate rapid color changes simulating Claude's status bar pulses
    const colorCodes = [
      '\x1b[38;5;196m', // Red
      '\x1b[38;5;202m', // Orange
      '\x1b[38;5;226m', // Yellow
      '\x1b[38;5;46m',  // Green
      '\x1b[38;5;21m',  // Blue
      '\x1b[38;5;201m', // Magenta
      '\x1b[38;5;51m',  // Cyan
    ];

    // Test 1: Rapid inline color changes (rainbow wave effect)
    await test.step('Rapid inline color changes', async () => {
      const message = 'Testing rapid color changes';
      let output = '';

      // Create 100 rapid color changes
      for (let i = 0; i < 100; i++) {
        const colorCode = colorCodes[i % colorCodes.length];
        output += `${colorCode}${message} ${i}\x1b[0m\r\n`;
      }

      await page.evaluate(async ({ sessionName, output }) => {
        const stressTest = (window as any).stressTest;
        if (!stressTest) throw new Error('Stress test not initialized');

        // Send all color changes at once
        await stressTest.sendChunk(sessionName, output);
      }, { sessionName, output });

      // Wait a bit to observe any flickering
      await page.waitForTimeout(1000);
    });

    // Test 2: Simulated status bar pulsing (overwriting same line)
    await test.step('Status bar pulsing simulation', async () => {
      const pulseSequence = async () => {
        for (let pulse = 0; pulse < 20; pulse++) {
          for (const colorCode of colorCodes) {
            const statusBar = `\r${colorCode}━━━ Processing... [${pulse}/${20}] ━━━\x1b[0m`;

            await page.evaluate(async ({ sessionName, chunk }) => {
              const stressTest = (window as any).stressTest;
              await stressTest.sendChunk(sessionName, chunk);
            }, { sessionName, chunk: statusBar });

            // Very rapid updates (10ms between colors)
            await page.waitForTimeout(10);
          }
        }
      };

      await pulseSequence();
      await page.waitForTimeout(500);
    });

    // Test 3: Background color wave (tmux status bar style)
    await test.step('Background color wave effect', async () => {
      const bgColors = [
        '\x1b[48;5;22m',  // Dark green
        '\x1b[48;5;28m',  // Green
        '\x1b[48;5;34m',  // Light green
        '\x1b[48;5;40m',  // Bright green
        '\x1b[48;5;34m',  // Light green
        '\x1b[48;5;28m',  // Green
      ];

      for (let wave = 0; wave < 10; wave++) {
        let statusLine = '\r\x1b[7m'; // Reverse video
        const segments = ['[session]', '0:claude*', '1:test-', 'CPU:45%', '14:23'];

        for (let i = 0; i < segments.length; i++) {
          const bgColor = bgColors[(wave + i) % bgColors.length];
          statusLine += `${bgColor}\x1b[38;5;255m ${segments[i]} `;
        }
        statusLine += '\x1b[0m'; // Reset

        await page.evaluate(async ({ sessionName, chunk }) => {
          const stressTest = (window as any).stressTest;
          await stressTest.sendChunk(sessionName, chunk);
        }, { sessionName, chunk: statusLine });

        await page.waitForTimeout(50); // 20fps update rate
      }
    });

    // Test 4: Rapid escape code sequences (stress test)
    await test.step('Escape code stress test', async () => {
      const escapeSequences = [
        '\x1b[1m',    // Bold
        '\x1b[3m',    // Italic
        '\x1b[4m',    // Underline
        '\x1b[5m',    // Blink
        '\x1b[7m',    // Reverse
        '\x1b[8m',    // Conceal
        '\x1b[9m',    // Strikethrough
      ];

      // Send rapid attribute changes
      let rapidChanges = '';
      for (let i = 0; i < 200; i++) {
        const seq = escapeSequences[i % escapeSequences.length];
        rapidChanges += `${seq}X\x1b[0m`;
      }
      rapidChanges += '\r\n';

      await page.evaluate(async ({ sessionName, chunk }) => {
        const stressTest = (window as any).stressTest;
        await stressTest.sendChunk(sessionName, chunk);
      }, { sessionName, chunk: rapidChanges });

      await page.waitForTimeout(500);
    });

    // Test 5: Claude-style thinking animation
    await test.step('Claude thinking animation', async () => {
      const thinkingFrames = [
        '⠋ Thinking...',
        '⠙ Thinking...',
        '⠹ Thinking...',
        '⠸ Thinking...',
        '⠼ Thinking...',
        '⠴ Thinking...',
        '⠦ Thinking...',
        '⠧ Thinking...',
        '⠇ Thinking...',
        '⠏ Thinking...',
      ];

      // Simulate rapid spinner updates with color changes
      for (let i = 0; i < 30; i++) {
        const frame = thinkingFrames[i % thinkingFrames.length];
        const color = colorCodes[i % colorCodes.length];
        const line = `\r${color}${frame}\x1b[0m (${i + 1}/30 tokens)`;

        await page.evaluate(async ({ sessionName, chunk }) => {
          const stressTest = (window as any).stressTest;
          await stressTest.sendChunk(sessionName, chunk);
        }, { sessionName, chunk: line });

        await page.waitForTimeout(33); // ~30fps
      }
    });

    // Capture a screenshot to see if there's visible artifacts
    await page.screenshot({
      path: `test-results/color-flicker-${Date.now()}.png`,
      fullPage: true
    });

    // Visual regression check
    const terminal = page.locator('.xterm-screen');
    await expect(terminal).toBeVisible();

    // Check that the terminal didn't crash or freeze
    const finalCheck = 'Final check - no flicker!\r\n';
    await page.evaluate(async ({ sessionName, chunk }) => {
      const stressTest = (window as any).stressTest;
      await stressTest.sendChunk(sessionName, chunk);
    }, { sessionName, chunk: finalCheck });

    await page.waitForTimeout(500);

    // Terminal should still be responsive
    await expect(terminal).toContainText('Final check');
  });

  test('should handle color pinwheel effect', async ({ page }) => {
    await navigateToStressTest(page);
    const sessionName = await launchSession(page, 'pinwheel');

    // Simulate the pinwheel/spinner effect during data output
    await test.step('Pinwheel during data output', async () => {
      const spinnerFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];
      const colors = [
        '\x1b[38;5;196m', // Red
        '\x1b[38;5;46m',  // Green
        '\x1b[38;5;21m',  // Blue
      ];

      // Simulate outputting data with a colored spinner
      const dataLines = Array.from({ length: 50 }, (_, i) =>
        `Processing line ${i}: ${Math.random().toString(36).substring(7)}`
      );

      for (let i = 0; i < dataLines.length; i++) {
        const spinner = spinnerFrames[i % spinnerFrames.length];
        const color = colors[i % colors.length];

        // Output data line
        await page.evaluate(async ({ sessionName, chunk }) => {
          const stressTest = (window as any).stressTest;
          await stressTest.sendChunk(sessionName, chunk);
        }, { sessionName, chunk: `${dataLines[i]}\r\n` });

        // Update spinner on same line
        const spinnerLine = `\r${color}${spinner} Processing... (${i + 1}/${dataLines.length})\x1b[0m`;
        await page.evaluate(async ({ sessionName, chunk }) => {
          const stressTest = (window as any).stressTest;
          await stressTest.sendChunk(sessionName, chunk);
        }, { sessionName, chunk: spinnerLine });

        await page.waitForTimeout(20); // Rapid updates
      }

      // Clear spinner line
      await page.evaluate(async ({ sessionName, chunk }) => {
        const stressTest = (window as any).stressTest;
        await stressTest.sendChunk(sessionName, chunk);
      }, { sessionName, chunk: '\r\x1b[K' }); // Clear line
    });

    // Check terminal is still functional
    const terminal = page.locator('.xterm-screen');
    await expect(terminal).toBeVisible();
  });
});