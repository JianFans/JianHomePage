import { createSSRApp } from 'vue'
import App from './App.vue'
import '../assets/css/main.css'

const source = document.getElementById('yujian-snapshot')
const snapshotURL = source?.getAttribute('data-src')
if (!source || !snapshotURL || !/^\/site\/snapshot-[a-f0-9]{64}\.json$/.test(snapshotURL)) {
  throw new Error('Missing content snapshot URL')
}
const snapshotPath: string = snapshotURL
const page = source.dataset.page === 'music' || source.dataset.page === 'video' || source.dataset.page === 'events' || source.dataset.page === 'moments' ? source.dataset.page : 'home'
let loading = false
let mounted = false
let retryButton: HTMLButtonElement | null = null

/** Keep a failed static snapshot request recoverable without hiding server-rendered content. */
function showSnapshotFailure() {
  const feedback = document.createElement('div')
  feedback.className = 'snapshot-hydration-error'
  feedback.setAttribute('role', 'alert')
  feedback.setAttribute('aria-live', 'assertive')

  const message = document.createElement('span')
  const isEnglish = document.documentElement.lang === 'en'
  message.textContent = isEnglish ? 'Page content could not be loaded.' : '页面内容加载失败。'

  retryButton = document.createElement('button')
  retryButton.type = 'button'
  retryButton.className = 'button button--secondary'
  retryButton.textContent = isEnglish ? 'Retry' : '重试'
  retryButton.addEventListener('click', () => { void hydrate() })

  feedback.append(message, retryButton)
  document.body.append(feedback)
}

/** Hydrate from the immutable, shared build snapshot after static markup is visible. */
async function hydrate() {
  if (loading || mounted) return
  loading = true
  if (retryButton) retryButton.disabled = true
  try {
    const response = await fetch(snapshotPath, { cache: 'force-cache', credentials: 'same-origin' })
    if (!response.ok) throw new Error('Could not load the published content snapshot')
    const snapshot = await response.json()
    mounted = true
    createSSRApp(App, { snapshot, page }).mount('#app')
    document.querySelector('.snapshot-hydration-error')?.remove()
  } catch {
    if (!retryButton) showSnapshotFailure()
  } finally {
    loading = false
    if (!mounted && retryButton) retryButton.disabled = false
  }
}

void hydrate().catch(error => console.error(error))
