import type { LocalizedText } from '@yujian/schema'

export type SupportedLocale = 'zh-CN' | 'en'

export function resolveLocalized(
  value: Partial<LocalizedText>,
  locale: SupportedLocale,
): string {
  return value[locale]?.trim() || value['zh-CN']?.trim() || ''
}
