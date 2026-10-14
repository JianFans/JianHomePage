import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { expect, it } from 'vitest'
import StudioDialog from '../../components/StudioDialog.vue'

it('moves focus into replacement dialog content and restores the original trigger', async () => {
  const host = defineComponent({
    components: { StudioDialog },
    setup: () => ({ open: ref(false), mode: ref('navigation') }),
    template: `<button data-testid="trigger" @click="open = true">Open</button>
      <StudioDialog :open="open" :content-key="mode" :title="mode" close-label="Close">
        <button :key="mode" :data-mode="mode" autofocus>Continue</button>
      </StudioDialog>`,
  })
  const wrapper = mount(host, { attachTo: document.body })
  try {
    const trigger = wrapper.get('[data-testid="trigger"]')
    ;(trigger.element as HTMLElement).focus()
    await trigger.trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[data-mode="navigation"]').element)
    wrapper.vm.mode = 'leave'
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[data-mode="leave"]').element)
    wrapper.vm.open = false
    await flushPromises()
    expect(document.activeElement).toBe(trigger.element)
  } finally { wrapper.unmount() }
})
