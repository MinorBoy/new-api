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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  preflightWorkbook,
  WorkbookPreflightError,
  type WorkbookPreflightResult,
} from './security'

type ConversionState =
  | { status: 'idle' }
  | { status: 'checking'; fileName: string }
  | { status: 'ready'; fileName: string; result: WorkbookPreflightResult }
  | { status: 'error'; fileName: string; message: string }

const panelStyle = {
  width: 'min(100% - 32px, 960px)',
  margin: '0 auto',
  padding: '48px 0 64px',
  fontFamily: 'system-ui, sans-serif',
  color: '#18212f',
} as const

export default function App() {
  const { t } = useTranslation()
  const [state, setState] = useState<ConversionState>({ status: 'idle' })

  async function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    if (!file) return
    setState({ status: 'checking', fileName: file.name })
    try {
      const result = await preflightWorkbook(file)
      setState({ status: 'ready', fileName: file.name, result })
    } catch (error: unknown) {
      const message =
        error instanceof WorkbookPreflightError
          ? t(`converter.errors.${error.code}`)
          : t('converter.errors.UNKNOWN')
      setState({ status: 'error', fileName: file.name, message })
    }
  }

  return (
    <div style={{ minHeight: '100vh', background: '#f5f7fa' }}>
      <main style={panelStyle}>
        <header style={{ marginBottom: 32 }}>
          <p style={{ margin: '0 0 8px', color: '#526173', fontSize: 13 }}>
            {t('Channel configuration')}
          </p>
          <h1
            style={{
              margin: 0,
              fontSize: 'clamp(28px, 5vw, 48px)',
              lineHeight: 1.1,
            }}
          >
            {t('Offline workbook converter')}
          </h1>
        </header>

        <section
          aria-labelledby='workbook-upload-title'
          style={{
            border: '1px solid #d8dee8',
            background: '#fff',
            padding: 24,
            borderRadius: 8,
          }}
        >
          <h2
            id='workbook-upload-title'
            style={{ margin: '0 0 16px', fontSize: 20 }}
          >
            {t('Workbook')}
          </h2>
          <label
            htmlFor='workbook-file'
            style={{ display: 'block', fontWeight: 600 }}
          >
            {t('Select an .xlsx file')}
          </label>
          <input
            id='workbook-file'
            type='file'
            accept='.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
            onChange={handleFileChange}
            style={{ display: 'block', marginTop: 12, maxWidth: '100%' }}
          />

          {state.status === 'checking' && (
            <p role='status' style={{ margin: '20px 0 0' }}>
              {t('Checking {{fileName}}', { fileName: state.fileName })}
            </p>
          )}

          {state.status === 'ready' && (
            <div role='status' style={{ marginTop: 20 }}>
              <p style={{ margin: '0 0 12px', fontWeight: 600 }}>
                {state.fileName}
              </p>
              <dl
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))',
                  gap: 12,
                  margin: 0,
                }}
              >
                <div>
                  <dt style={{ color: '#526173', fontSize: 13 }}>
                    {t('Worksheets')}
                  </dt>
                  <dd style={{ margin: '4px 0 0', fontSize: 22 }}>
                    {state.result.sheetCount}
                  </dd>
                </div>
                <div>
                  <dt style={{ color: '#526173', fontSize: 13 }}>
                    {t('Rows')}
                  </dt>
                  <dd style={{ margin: '4px 0 0', fontSize: 22 }}>
                    {state.result.rowCount}
                  </dd>
                </div>
                <div>
                  <dt style={{ color: '#526173', fontSize: 13 }}>
                    {t('Entities')}
                  </dt>
                  <dd style={{ margin: '4px 0 0', fontSize: 22 }}>
                    {state.result.entityCount}
                  </dd>
                </div>
              </dl>
            </div>
          )}

          {state.status === 'error' && (
            <p role='alert' style={{ margin: '20px 0 0', color: '#b42318' }}>
              {state.message}
            </p>
          )}
        </section>
      </main>
    </div>
  )
}
