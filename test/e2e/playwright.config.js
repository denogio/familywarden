const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
  testDir: '.',
  fullyParallel: false,
  // One router, shared state: never run files in parallel
  workers: 1,
  timeout: 30000,
  expect: { timeout: 10000 },
  retries: 0,
  reporter: 'list',
  use: {
    headless: true,
    viewport: { width: 1280, height: 720 },
    ignoreHTTPSErrors: true,
    // Collect trace on failure for debugging
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: undefined, // scripts/test-e2e.sh (container) or scripts/qemu-dev.sh
});
