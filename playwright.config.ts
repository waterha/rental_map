import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.ts',
  timeout: 45000,
  expect: { timeout: 12000 },
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.TEST_BASE_URL || 'http://127.0.0.1:8080',
    channel: 'msedge',
    headless: true,
    viewport: { width: 1512, height: 1080 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
})
