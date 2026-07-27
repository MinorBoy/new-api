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
import { Download, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { serializeImportDocument, type ConfigImportDocument } from '../document'

export interface DownloadActionsProps {
  document: ConfigImportDocument
  formalDownloadDisabled: boolean
  onClear: () => void
}

function download(contents: string, fileName: string): void {
  const href = URL.createObjectURL(
    new Blob([contents], { type: 'application/json;charset=utf-8' })
  )
  const link = window.document.createElement('a')
  link.href = href
  link.download = fileName
  link.click()
  URL.revokeObjectURL(href)
}

export function DownloadActions(props: DownloadActionsProps) {
  const { t } = useTranslation()

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 20 }}>
      <button
        disabled={props.formalDownloadDisabled}
        onClick={() =>
          download(
            serializeImportDocument(props.document),
            'channel-config-import.json'
          )
        }
        type='button'
      >
        <Download aria-hidden='true' size={16} /> {t('Download JSON')}
      </button>
      <button
        onClick={() =>
          download(
            `${JSON.stringify(props.document.issues, null, 2)}\n`,
            'channel-config-issues.json'
          )
        }
        type='button'
      >
        <Download aria-hidden='true' size={16} /> {t('Download issue report')}
      </button>
      <button onClick={props.onClear} type='button'>
        <Trash2 aria-hidden='true' size={16} /> {t('Clear')}
      </button>
    </div>
  )
}
