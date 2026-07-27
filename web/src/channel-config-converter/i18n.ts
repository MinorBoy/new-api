/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { createInstance } from 'i18next'

import en from '../i18n/locales/en.json'
import fr from '../i18n/locales/fr.json'
import ja from '../i18n/locales/ja.json'
import ru from '../i18n/locales/ru.json'
import vi from '../i18n/locales/vi.json'
import zhTW from '../i18n/locales/zh-TW.json'
import zhCN from '../i18n/locales/zh.json'

const resources = {
  en: { translation: en },
  zhCN: { translation: zhCN },
  fr: { translation: fr },
  ja: { translation: ja },
  ru: { translation: ru },
  vi: { translation: vi },
  zhTW: { translation: zhTW },
} as const

function detectLanguage(): keyof typeof resources {
  const language =
    typeof navigator === 'undefined' ? 'en' : navigator.language.toLowerCase()
  if (language.startsWith('zh-tw') || language.startsWith('zh-hk')) {
    return 'zhTW'
  }
  if (language.startsWith('zh')) {
    return 'zhCN'
  }
  const match = Object.keys(resources).find((key) =>
    language.startsWith(key.toLowerCase())
  )
  return (match as keyof typeof resources | undefined) ?? 'en'
}

export const converterI18n = createInstance()

export const converterI18nReady = converterI18n.init({
  resources,
  lng: detectLanguage(),
  fallbackLng: 'en',
  supportedLngs: Object.keys(resources),
  load: 'currentOnly',
  ns: ['translation'],
  defaultNS: 'translation',
  nsSeparator: false,
  interpolation: {
    escapeValue: false,
  },
})
