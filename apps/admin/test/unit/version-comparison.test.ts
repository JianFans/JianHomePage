import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import VersionComparison from '../../components/VersionComparison.vue'
import type { AdminVersion, AdminVersionSummary } from '../../utils/admin-api'

const versions: AdminVersionSummary[] = ['one', 'two', 'three'].map((id, index) => ({ id, name: `Saved ${id}`, status: 'draft', revision: index + 1, checksum: `sha256:${id}` }))
const saved = (id: string, brand = id): AdminVersion => ({ ...versions.find(item => item.id === id)!, snapshot: { site: { brand } } })
const response = (id: string, brand = id) => new Response(JSON.stringify(saved(id, brand)), { status: 200 })
const props = { versions, currentVersionId: 'one', apiBaseUrl: 'https://cms.example', csrfToken: 'session', locale: 'en' as const, active: true }
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}
afterEach(() => vi.unstubAllGlobals())

describe('read-only version comparison', () => {
  it('announces pending reads outside busy content', async () => {
    const pending = deferred<Response>()
    vi.stubGlobal('fetch', () => pending.promise)
    const wrapper = mount(VersionComparison, { props })
    try {
      expect(wrapper.findAll('[role="status"]')).toHaveLength(2)
      for (const status of wrapper.findAll('[role="status"]')) {
        expect(status.element.closest('[aria-busy="true"]')).toBeNull()
      }
    } finally { wrapper.unmount(); pending.resolve(response('one')) }
  })

  it('reads two saved versions using GET and never emits an editing or version-switch event', async () => {
    const requests: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal('fetch', async (input: string, init?: RequestInit) => {
      requests.push({ url: input, init })
      return response(input.split('/').pop()!)
    })
    const wrapper = mount(VersionComparison, { props })
    try {
      await flushPromises()
      expect(wrapper.text()).toContain('/site/brand')
      expect(wrapper.text()).toContain('Saved one')
      expect(wrapper.text()).toContain('r2')
      expect(requests.map(item => [item.url, item.init?.method || 'GET'])).toEqual([
        ['https://cms.example/api/v1/versions/one', 'GET'], ['https://cms.example/api/v1/versions/two', 'GET'],
      ])
      expect(wrapper.emitted('select')).toBeUndefined()
      expect(wrapper.emitted('update')).toBeUndefined()
      expect(versions[0]!.revision).toBe(1)
    } finally { wrapper.unmount() }
  })

  it('keeps each side independent and retries only the failed read', async () => {
    let attempts = 0
    vi.stubGlobal('fetch', async (input: string) => {
      const id = input.split('/').pop()!
      if (id === 'two' && ++attempts === 1) return new Response('{}', { status: 503 })
      return response(id)
    })
    const wrapper = mount(VersionComparison, { props })
    try {
      await flushPromises()
      expect(wrapper.get('[data-testid="compare-left-loaded"]').text()).toContain('Saved one')
      expect(wrapper.get('[data-testid="compare-right-error"]').text()).toContain('load')
      await wrapper.get('[data-testid="compare-right-retry"]').trigger('click')
      await flushPromises()
      expect(wrapper.find('[data-testid="compare-right-error"]').exists()).toBe(false)
      expect(wrapper.text()).toContain('/site/brand')
    } finally { wrapper.unmount() }
  })

  it('aborts old selections and ignores late successes and stale unauthorized responses', async () => {
    const old = deferred<Response>()
    let oldSignal: AbortSignal | undefined
    vi.stubGlobal('fetch', (input: string, init?: RequestInit) => {
      if (input.endsWith('/two')) { oldSignal = init?.signal as AbortSignal; return old.promise }
      return Promise.resolve(response(input.split('/').pop()!))
    })
    const wrapper = mount(VersionComparison, { props })
    try {
      await flushPromises()
      await wrapper.get('[data-testid="compare-right-select"]').setValue('three')
      await flushPromises()
      expect(oldSignal?.aborted).toBe(true)
      old.resolve(new Response('{}', { status: 401 }))
      await flushPromises()
      expect(wrapper.get('[data-testid="compare-right-loaded"]').text()).toContain('Saved three')
      expect(wrapper.emitted('unauthorized')).toBeUndefined()
      expect(wrapper.find('[data-testid="compare-right-error"]').exists()).toBe(false)
    } finally { wrapper.unmount() }
  })

  it('ignores old successful payloads after the selection has changed', async () => {
    const old = deferred<Response>()
    vi.stubGlobal('fetch', (input: string) => input.endsWith('/two') ? old.promise : Promise.resolve(response(input.split('/').pop()!)))
    const wrapper = mount(VersionComparison, { props })
    try {
      await wrapper.get('[data-testid="compare-right-select"]').setValue('three')
      await flushPromises()
      old.resolve(response('two', 'Stale content'))
      await flushPromises()
      expect(wrapper.text()).not.toContain('Stale content')
      expect(wrapper.get('[data-testid="compare-right-loaded"]').text()).toContain('Saved three')
    } finally { wrapper.unmount() }
  })

  it('does not read while inactive and invalidates pending reads on closure and unmount', async () => {
    const pending = deferred<Response>()
    const signals: AbortSignal[] = []
    vi.stubGlobal('fetch', (_input: string, init?: RequestInit) => { signals.push(init!.signal as AbortSignal); return pending.promise })
    const wrapper = mount(VersionComparison, { props: { ...props, active: false } })
    expect(signals).toHaveLength(0)
    await wrapper.setProps({ active: true })
    expect(signals).toHaveLength(2)
    await wrapper.setProps({ active: false })
    expect(signals.every(signal => signal.aborted)).toBe(true)
    pending.resolve(new Response('{}', { status: 401 }))
    await flushPromises()
    expect(wrapper.emitted('unauthorized')).toBeUndefined()
    await wrapper.setProps({ active: true })
    wrapper.unmount()
    expect(signals.every(signal => signal.aborted)).toBe(true)
  })

  it('invalidates both sides when the API connection or session changes', async () => {
    const old = deferred<Response>()
    vi.stubGlobal('fetch', (input: string) => input.startsWith('https://cms.example') ? old.promise : Promise.resolve(response(input.split('/').pop()!, 'New connection')))
    const wrapper = mount(VersionComparison, { props })
    try {
      await wrapper.setProps({ apiBaseUrl: 'https://other.example', csrfToken: 'new-session' })
      await flushPromises()
      old.resolve(new Response('{}', { status: 401 }))
      await flushPromises()
      expect(wrapper.get('[data-testid="compare-left-loaded"]').text()).toContain('Saved one')
      expect(wrapper.emitted('unauthorized')).toBeUndefined()
      expect(wrapper.text()).toContain('No differences')
    } finally { wrapper.unmount() }
  })

  it('forwards an active unauthorized response and supports version-list pagination', async () => {
    vi.stubGlobal('fetch', async () => new Response('{}', { status: 401 }))
    const wrapper = mount(VersionComparison, { props: { ...props, nextCursor: 'next' } })
    try {
      await flushPromises()
      expect(wrapper.emitted('unauthorized')!.length).toBeGreaterThan(0)
      await wrapper.get('[data-testid="compare-more"]').trigger('click')
      expect(wrapper.emitted('more')).toEqual([[]])
      await wrapper.setProps({ listing: true })
      expect(wrapper.get('[data-testid="compare-more"]').attributes('disabled')).toBeDefined()
    } finally { wrapper.unmount() }
  })

  it('retains selected saved-version options when the surrounding history list refreshes', async () => {
    vi.stubGlobal('fetch', async (input: string) => response(input.split('/').pop()!))
    const wrapper = mount(VersionComparison, { props })
    try {
      await flushPromises()
      await wrapper.setProps({ versions: [versions[2]!] })
      expect(wrapper.get<HTMLSelectElement>('[data-testid="compare-left-select"]').element.value).toBe('one')
      expect(wrapper.get<HTMLSelectElement>('[data-testid="compare-right-select"]').element.value).toBe('two')
      expect(wrapper.text()).toContain('/site/brand')
    } finally { wrapper.unmount() }
  })

  it('renders content as text with expandable values and bounded change pages', async () => {
    const malicious = '<img src=x onerror=alert(1)>'
    vi.stubGlobal('fetch', async (input: string) => new Response(JSON.stringify({ ...saved(input.split('/').pop()!), snapshot: { site: Object.fromEntries(Array.from({ length: 180 }, (_, index) => [`field${index}`, input.endsWith('/one') ? 'Old' : malicious.repeat(100)])) } }), { status: 200 }))
    const wrapper = mount(VersionComparison, { props })
    try {
      await flushPromises()
      expect(wrapper.findAll('[data-testid="compare-change"]')).toHaveLength(50)
      expect(wrapper.find('img').exists()).toBe(false)
      const expanded = wrapper.get('[data-testid="compare-value-details"]')
      expect(expanded.find('pre').exists()).toBe(false)
      ;(expanded.element as HTMLDetailsElement).open = true
      await expanded.trigger('toggle')
      expect(expanded.get('pre').text()).toContain(malicious)
      await wrapper.get('[data-testid="compare-changes-more"]').trigger('click')
      expect(wrapper.findAll('[data-testid="compare-change"]')).toHaveLength(100)
    } finally { wrapper.unmount() }
  })
})
