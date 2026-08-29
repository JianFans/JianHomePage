import type { SupportedLocale } from '../utils/localized'
import { useI18n } from 'vue-i18n'

export const DEFAULT_LOCALE: SupportedLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'yujian:locale'

interface LocaleDetectionInput {
  stored: string | null
  browser: readonly string[]
}

interface LocaleDetectionResult {
  locale: SupportedLocale
  notify: boolean
}

function isSupportedLocale(value: string): value is SupportedLocale {
  return value === 'zh-CN' || value === 'en'
}

function normalizeBrowserLocale(value: string): SupportedLocale | null {
  const normalized = value.toLowerCase()

  if (normalized === 'en' || normalized.startsWith('en-')) {
    return 'en'
  }

  if (
    normalized === 'zh-cn'
    || normalized === 'zh-hans'
    || normalized.startsWith('zh-hans-')
  ) {
    return 'zh-CN'
  }

  return null
}

export function detectLocale({
  stored,
  browser,
}: LocaleDetectionInput): LocaleDetectionResult {
  if (stored && isSupportedLocale(stored)) {
    return { locale: stored, notify: false }
  }

  for (const language of browser) {
    const browserLocale = normalizeBrowserLocale(language)
    if (browserLocale) {
      return {
        locale: browserLocale,
        notify: browserLocale !== DEFAULT_LOCALE,
      }
    }
  }

  return { locale: DEFAULT_LOCALE, notify: false }
}

export function useLocale() {
  const { locale } = useI18n({ useScope: 'global' })
  const noticeVisible = useState('locale-notice', () => false)

  function selectLocale(nextLocale: SupportedLocale) {
    locale.value = nextLocale
    noticeVisible.value = false

    if (import.meta.client) {
      document.documentElement.lang = nextLocale
      localStorage.setItem(LOCALE_STORAGE_KEY, nextLocale)
    }
  }

  return {
    locale,
    noticeVisible,
    selectLocale,
  }
}
