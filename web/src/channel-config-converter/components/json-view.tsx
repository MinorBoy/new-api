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
import { useTranslation } from 'react-i18next'

import { serializeImportDocument, type ConfigImportDocument } from '../document'

export interface JsonViewProps {
  document: ConfigImportDocument
}

export function JsonView(props: JsonViewProps) {
  const { t } = useTranslation()

  return (
    <section aria-labelledby='converter-json-title'>
      <h2
        id='converter-json-title'
        style={{ fontSize: 18, margin: '0 0 14px' }}
      >
        {t('JSON')}
      </h2>
      <pre
        style={{
          background: '#18212f',
          color: '#e6edf5',
          margin: 0,
          maxHeight: 520,
          overflow: 'auto',
          padding: 16,
        }}
      >
        {serializeImportDocument(props.document)}
      </pre>
    </section>
  )
}
