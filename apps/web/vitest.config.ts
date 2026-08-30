import { defineVitestConfig } from '@nuxt/test-utils/config'
import { coverageConfig } from '../../vitest.coverage.mjs'

export default defineVitestConfig({
  test: {
    environment: 'nuxt',
    globals: true,
    include: ['test/unit/**/*.test.ts'],
    setupFiles: ['./test/setup.ts'],
    coverage: coverageConfig('web', [
      'app.vue',
      'components/**/*.vue',
      'composables/**/*.ts',
      'pages/**/*.vue',
      'plugins/**/*.ts',
      'server/**/*.ts',
      'utils/**/*.ts',
    ]),
  },
})
