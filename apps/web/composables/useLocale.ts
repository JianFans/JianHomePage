import type { SupportedLocale } from '../utils/localized'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

export const DEFAULT_LOCALE: SupportedLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'yujian:locale'

type ReadableStorage = Pick<Storage, 'getItem'>
type WritableStorage = Pick<Storage, 'setItem'>

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

  if (normalized === 'zh' || normalized.startsWith('zh-')) {
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

export function readLocalePreference(storage?: ReadableStorage): string | null {
  try {
    return (storage ?? window.localStorage).getItem(LOCALE_STORAGE_KEY)
  } catch {
    return null
  }
}

export function writeLocalePreference(storage: WritableStorage | undefined, locale: SupportedLocale): void {
  try {
    (storage ?? window.localStorage).setItem(LOCALE_STORAGE_KEY, locale)
  } catch {
    // Storage may be unavailable in privacy modes; the active in-memory locale remains valid.
  }
}

export function useLocale() {
  const { locale: i18nLocale } = useI18n({ useScope: 'global' })
  const locale = computed<SupportedLocale>({
    get: () => i18nLocale.value === 'en' ? 'en' : DEFAULT_LOCALE,
    set: (nextLocale) => {
      i18nLocale.value = nextLocale
    },
  })
  const noticeVisible = useState('locale-notice', () => false)

  function selectLocale(nextLocale: SupportedLocale) {
    locale.value = nextLocale
    noticeVisible.value = false

    if (import.meta.client) {
      document.documentElement.lang = nextLocale
      writeLocalePreference(undefined, nextLocale)
    }
  }

  return {
    locale,
    noticeVisible,
    selectLocale,
  }
}
