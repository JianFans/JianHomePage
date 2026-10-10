import { defineVitestConfig } from '@nuxt/test-utils/config'
import { coverageConfig } from '../../vitest.coverage.mjs'

export default defineVitestConfig({
  test: {
    environment: 'nuxt',
    include: ['test/unit/**/*.test.ts'],
    coverage: coverageConfig('admin', [
      'app.vue',
      'components/**/*.vue',
      'composables/**/*.ts',
      'pages/**/*.vue',
      'utils/**/*.ts',
    ]),
  },
})
