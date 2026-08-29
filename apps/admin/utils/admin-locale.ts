export type AdminLocale = 'zh-CN' | 'en'

const ADMIN_LOCALE_KEY = 'yujian:admin-locale'

type ReadableStorage = Pick<Storage, 'getItem'>
type WritableStorage = Pick<Storage, 'setItem'>

export function resolveAdminLocale(
  storage: ReadableStorage | undefined,
  browserLanguage: string,
): AdminLocale {
  try {
    const stored = (storage ?? window.localStorage).getItem(ADMIN_LOCALE_KEY)
    if (stored === 'en' || stored === 'zh-CN') return stored
  } catch {
    // Browser privacy settings may disable storage; language detection still works.
  }
  return browserLanguage.toLowerCase().startsWith('en') ? 'en' : 'zh-CN'
}

export function persistAdminLocale(locale: AdminLocale, storage?: WritableStorage): void {
  try {
    (storage ?? window.localStorage).setItem(ADMIN_LOCALE_KEY, locale)
  } catch {
    // Keep the active in-memory preference when storage is unavailable.
  }
}
