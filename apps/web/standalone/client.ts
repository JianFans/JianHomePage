import { createSSRApp } from 'vue'
import App from './App.vue'
import '../assets/css/main.css'

const source = document.getElementById('yujian-snapshot')
if (!source?.textContent) throw new Error('Missing content snapshot')
const page = source.dataset.page === 'music' ? 'music' : 'home'
createSSRApp(App, { snapshot: JSON.parse(source.textContent), page }).mount('#app')
