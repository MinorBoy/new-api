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
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import App from './app'
import { converterI18n, converterI18nReady } from './i18n'

const rootElement = document.querySelector<HTMLElement>('#root')
if (!rootElement) {
  throw new Error('Converter root element not found')
}

void converterI18nReady.then(() => {
  ReactDOM.createRoot(rootElement).render(
    <StrictMode>
      <I18nextProvider i18n={converterI18n}>
        <App />
      </I18nextProvider>
    </StrictMode>
  )
})
