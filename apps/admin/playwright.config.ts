import { defineConfig, devices } from '@playwright/test'

const port = 49318
const baseURL = process.env.PLAYWRIGHT_ADMIN_BASE_URL || `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: './test/e2e',
  outputDir: './test-results',
  fullyParallel: false,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
  webServer: process.env.PLAYWRIGHT_ADMIN_BASE_URL ? undefined : {
    command: 'node .output/server/index.mjs',
    url: baseURL,
    env: { NITRO_HOST: '127.0.0.1', NITRO_PORT: String(port) },
    stdout: 'pipe',
    stderr: 'pipe',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
