import { defineConfig, devices } from '@playwright/test'

const port = 49317
const baseURL = `http://localhost:${port}`

export default defineConfig({
  testDir: './test/e2e',
  outputDir: './test-results',
  fullyParallel: false,
  reporter: 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
  webServer: {
    command: `pnpm dev --host localhost --port ${port}`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
