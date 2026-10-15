import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import App from '../../app.vue'

// Studio interactions are covered by studio-shell.test.ts. Simultaneously
// open workbenches have been replaced by one active module and shared save.
it('mounts the Nuxt page outlet', () => {
  const wrapper = mount(App, {
    global: { stubs: { NuxtPage: { template: '<main data-testid="nuxt-page" />' } } },
  })
  expect(wrapper.find('[data-testid="nuxt-page"]').exists()).toBe(true)
  wrapper.unmount()
})
