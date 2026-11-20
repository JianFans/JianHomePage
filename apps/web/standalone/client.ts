import { createSSRApp } from 'vue'
import App from './App.vue'
import '../assets/css/main.css'

const source = document.getElementById('yujian-snapshot')
if (!source?.textContent) throw new Error('Missing content snapshot')
const page = source.dataset.page === 'music' || source.dataset.page === 'video' || source.dataset.page === 'events' ? source.dataset.page : 'home'
createSSRApp(App, { snapshot: JSON.parse(source.textContent), page }).mount('#app')
