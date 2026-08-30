import { defineConfig } from 'vitest/config'
import { coverageConfig } from '../../vitest.coverage.mjs'

export default defineConfig({
  test: {
    include: ['test/**/*.test.ts'],
    coverage: coverageConfig('schema', ['src/**/*.ts'], ['src/generated.ts']),
  },
})
