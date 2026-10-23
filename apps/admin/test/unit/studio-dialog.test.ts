import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { expect, it, vi } from 'vitest'
import StudioDialog from '../../components/StudioDialog.vue'

it('initializes an initially open dialog and restores focus and scrolling on unmount', async () => {
  const trigger = document.createElement('button')
  document.body.append(trigger)
  trigger.focus()
  const originalOverflow = document.body.style.overflow
  document.body.style.overflow = 'auto'
  const wrapper = mount(StudioDialog, {
    attachTo: document.body,
    props: { open: true, title: 'Assets', closeLabel: 'Close' },
    slots: { default: '<input autofocus />' },
  })
  try {
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('input').element)
    expect(document.body.style.overflow).toBe('hidden')
    wrapper.unmount()
    expect(document.activeElement).toBe(trigger)
    expect(document.body.style.overflow).toBe('auto')
  } finally {
    if (wrapper.exists()) wrapper.unmount()
    trigger.remove()
    document.body.style.overflow = originalOverflow
  }
})

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

it('retains the original trigger across temporary hiding of a mounted upload dialog', async () => {
  const trigger = document.createElement('button')
  const restore = document.createElement('button')
  document.body.append(trigger, restore)
  trigger.focus()
  const originalOverflow = document.body.style.overflow
  const wrapper = mount(StudioDialog, {
    attachTo: document.body,
    props: { open: true, keepMounted: true, title: 'Upload', closeLabel: 'Close' },
    slots: { default: '<input autofocus />' },
  })
  try {
    await flushPromises()
    const input = wrapper.get('input').element
    await wrapper.setProps({ open: false })
    expect(wrapper.get('input').isVisible()).toBe(false)
    expect(document.body.style.overflow).toBe(originalOverflow)
    restore.focus()
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(wrapper.get('input').element).toBe(input)
    expect(document.activeElement).toBe(input)
    wrapper.unmount()
    expect(document.activeElement).toBe(trigger)
  } finally {
    if (wrapper.exists()) wrapper.unmount()
    trigger.remove(); restore.remove()
    document.body.style.overflow = originalOverflow
  }
})

it('guards iframe Escape while busy and removes listeners on close and unmount', async () => {
  const close = vi.fn()
  const wrapper = mount(StudioDialog, {
    attachTo: document.body,
    props: { open: false, title: 'Preview', closeLabel: 'Close', busy: true, onClose: close },
    // srcdoc creates an isolated same-origin document without enabling network page loading.
    slots: { default: '<iframe srcdoc="<button>Preview</button>" />' },
  })
  try {
    await wrapper.setProps({ open: true })
    const frame = wrapper.get('iframe')
    const childDocument = (frame.element as HTMLIFrameElement).contentDocument!
    expect(childDocument).not.toBeNull()
    await frame.trigger('load')
    childDocument.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    expect(wrapper.emitted('close')).toBeUndefined()
    await wrapper.setProps({ busy: false })
    childDocument.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    const removeListener = vi.spyOn(childDocument, 'removeEventListener')
    frame.element.remove()
    await flushPromises()
    expect(removeListener).toHaveBeenCalledWith('keydown', expect.any(Function))
    childDocument.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(close).toHaveBeenCalledOnce()
    await wrapper.setProps({ open: false })
    childDocument.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    await wrapper.setProps({ open: true })
    const reopenedDocument = (wrapper.get('iframe').element as HTMLIFrameElement).contentDocument!
    await wrapper.get('iframe').trigger('load')
    wrapper.unmount()
    reopenedDocument.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(close).toHaveBeenCalledOnce()
  } finally { if (wrapper.exists()) wrapper.unmount() }
})
