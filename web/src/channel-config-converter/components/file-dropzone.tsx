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
import { Upload } from 'lucide-react'
import type { ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

export interface FileDropzoneProps {
  error?: string
  fileName?: string
  isChecking: boolean
  onFileChange: (event: ChangeEvent<HTMLInputElement>) => void
}

export function FileDropzone(props: FileDropzoneProps) {
  const { t } = useTranslation()

  return (
    <section
      aria-labelledby='workbook-upload-title'
      style={{ padding: '24px 0' }}
    >
      <h2
        id='workbook-upload-title'
        style={{ fontSize: 18, margin: '0 0 14px' }}
      >
        {t('Workbook')}
      </h2>
      <label
        htmlFor='workbook-file'
        style={{
          alignItems: 'center',
          display: 'inline-flex',
          fontWeight: 600,
          gap: 8,
        }}
      >
        <Upload aria-hidden='true' size={18} />
        {t('Select an .xlsx file')}
      </label>
      <input
        id='workbook-file'
        type='file'
        accept='.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        disabled={props.isChecking}
        onChange={props.onFileChange}
        style={{ display: 'block', marginTop: 12, maxWidth: '100%' }}
      />
      {props.isChecking && props.fileName && (
        <p role='status' style={{ marginBottom: 0 }}>
          {t('Checking {{fileName}}', { fileName: props.fileName })}
        </p>
      )}
      {props.error && (
        <p role='alert' style={{ color: '#b42318', marginBottom: 0 }}>
          {props.error}
        </p>
      )}
    </section>
  )
}
