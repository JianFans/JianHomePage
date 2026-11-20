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

/** Hydrate from the immutable, shared build snapshot after static markup is visible. */
async function hydrate() {
  const response = await fetch(snapshotPath, { cache: 'force-cache', credentials: 'same-origin' })
  if (!response.ok) throw new Error('Could not load the published content snapshot')
  const snapshot = await response.json()
  createSSRApp(App, { snapshot, page }).mount('#app')
}

void hydrate().catch(error => console.error(error))
