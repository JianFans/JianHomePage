import { createI18n } from 'vue-i18n'
import en from '../locales/en'
import zhCN from '../locales/zh-CN'
import {
  detectLocale,
  LOCALE_STORAGE_KEY,
} from '../composables/useLocale'

export default defineNuxtPlugin((nuxtApp) => {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    fallbackLocale: 'zh-CN',
    messages: {
      'zh-CN': zhCN,
      en,
    },
  })

  nuxtApp.vueApp.use(i18n)
  const noticeVisible = useState('locale-notice', () => false)

  nuxtApp.hook('app:mounted', () => {
    const detected = detectLocale({
      stored: localStorage.getItem(LOCALE_STORAGE_KEY),
      browser: navigator.languages.length
        ? navigator.languages
        : [navigator.language],
    })

    i18n.global.locale.value = detected.locale
    document.documentElement.lang = detected.locale
    localStorage.setItem(LOCALE_STORAGE_KEY, detected.locale)
    noticeVisible.value = detected.notify
  })
})
