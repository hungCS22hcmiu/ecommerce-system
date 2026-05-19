import { defineConfig, devices } from '@playwright/test'

// Phase 5 E2E — Chromium only, runs against the docker compose frontend (port 3001).
// JSON reporter so the aggregator can parse pass/fail per spec.

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  retries: 1,
  reporter: [
    ['list'],
    ['json', { outputFile: 'test-results/playwright.json' }],
  ],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3001',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})
