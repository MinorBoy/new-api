import { useTranslation } from 'react-i18next'

import type { DocLocale } from '../types'

/**
 * Resolve the active documentation locale from the current i18next language.
 * Chinese variants (zh, zh-CN, zh-TW) map to `zh`; everything else falls back
 * to `en`, which is the only fully-authored language set for every page.
 */
export function useDocLocale(): DocLocale {
  const { i18n } = useTranslation()
  return i18n.language.startsWith('zh') ? 'zh' : 'en'
}
