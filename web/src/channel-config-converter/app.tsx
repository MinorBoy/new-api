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
import { useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { V1WorkbookAdapter } from './adapters/v1'
import { V2WorkbookAdapter } from './adapters/v2'
import { ConverterHeader } from './components/converter-header'
import { DownloadActions } from './components/download-actions'
import { EntityTable } from './components/entity-table'
import { FileDropzone } from './components/file-dropzone'
import { IssueView } from './components/issue-view'
import { JsonView } from './components/json-view'
import { SummaryView } from './components/summary-view'
import { buildImportDocument, type ImportDocumentResult } from './document'
import { WorkbookContractError } from './schema'
import { preflightWorkbook, WorkbookPreflightError } from './security'
import { loadWorkbookSnapshot } from './workbook'

type ConversionState =
  | { status: 'idle' }
  | { status: 'checking'; fileName: string }
  | { status: 'ready'; fileName: string; result: WorkbookConversion }
  | { status: 'error'; fileName: string; message: string }

const tabs = [
  'Overview',
  'Channels and lines',
  'Model SKUs',
  'Sale pricing',
  'Channel costs',
  'Model mappings and routing',
  'Issues',
  'JSON',
] as const

type Tab = (typeof tabs)[number]

export type WorkbookConversion = ImportDocumentResult

export interface AppProps {
  convertFile?: (file: File) => Promise<WorkbookConversion>
}

async function convertWorkbook(file: File): Promise<WorkbookConversion> {
  await preflightWorkbook(file)
  const snapshot = await loadWorkbookSnapshot(await file.arrayBuffer())
  const adapters = [new V2WorkbookAdapter(), new V1WorkbookAdapter()]
  const adapter = adapters.find(
    (candidate) => candidate.matches(snapshot).matched
  )
  if (!adapter) {
    throw new WorkbookContractError(
      'UNSUPPORTED_TEMPLATE',
      'No supported workbook template matched.'
    )
  }
  return buildImportDocument({
    extracted: adapter.extract(snapshot),
    sourceBytes: new Uint8Array(await file.arrayBuffer()),
    sourceFileName: file.name,
  })
}

export default function App(props: AppProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<ConversionState>({ status: 'idle' })
  const [tab, setTab] = useState<Tab>('Overview')

  async function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    if (!file) return
    setState({ status: 'checking', fileName: file.name })
    try {
      const result = await (props.convertFile ?? convertWorkbook)(file)
      setState({ status: 'ready', fileName: file.name, result })
      setTab('Overview')
    } catch (error: unknown) {
      let message = t('converter.errors.UNKNOWN')
      if (error instanceof WorkbookPreflightError) {
        message = t(`converter.errors.${error.code}`)
      } else if (error instanceof WorkbookContractError) {
        message = t('The workbook does not match a supported template.')
      }
      setState({ status: 'error', fileName: file.name, message })
    }
  }

  return (
    <div style={{ minHeight: '100vh', background: '#f5f7fa' }}>
      <main
        style={{
          boxSizing: 'border-box',
          color: '#18212f',
          fontFamily: 'system-ui, sans-serif',
          margin: '0 auto',
          padding: '40px 16px 64px',
          width: 'min(100%, 1180px)',
        }}
      >
        <ConverterHeader />
        <FileDropzone
          error={state.status === 'error' ? state.message : undefined}
          fileName={state.status === 'idle' ? undefined : state.fileName}
          isChecking={state.status === 'checking'}
          onFileChange={handleFileChange}
        />
        {state.status === 'ready' && (
          <section
            aria-label={t('Conversion preview')}
            style={{ borderTop: '1px solid #d7dee8', paddingTop: 20 }}
          >
            <div
              role='tablist'
              style={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 4,
                marginBottom: 20,
                maxWidth: '100%',
              }}
            >
              {tabs.map((item) => (
                <button
                  aria-selected={tab === item}
                  key={item}
                  onClick={() => setTab(item)}
                  role='tab'
                  type='button'
                >
                  {t(item)}
                </button>
              ))}
            </div>
            {tab === 'Overview' && (
              <SummaryView document={state.result.document} />
            )}
            {tab === 'Channels and lines' && (
              <EntityTable
                entities={[
                  ...state.result.document.entities.channels,
                  ...state.result.document.entities.channel_lines,
                ]}
                title='Channels and lines'
              />
            )}
            {tab === 'Model SKUs' && (
              <EntityTable
                entities={state.result.document.entities.model_skus}
                title='Model SKUs'
              />
            )}
            {tab === 'Sale pricing' && (
              <EntityTable
                entities={state.result.document.entities.sale_proposals}
                title='Sale pricing'
              />
            )}
            {tab === 'Channel costs' && (
              <EntityTable
                entities={state.result.document.entities.cost_rule_drafts}
                title='Channel costs'
              />
            )}
            {tab === 'Model mappings and routing' && (
              <EntityTable
                entities={[
                  ...state.result.document.entities.model_mappings,
                  ...state.result.document.entities.route_blueprints,
                ]}
                title='Model mappings and routing'
              />
            )}
            {tab === 'Issues' && (
              <IssueView issues={state.result.document.issues} />
            )}
            {tab === 'JSON' && <JsonView document={state.result.document} />}
            <DownloadActions
              document={state.result.document}
              formalDownloadDisabled={state.result.hasFailures}
              onClear={() => setState({ status: 'idle' })}
            />
          </section>
        )}
      </main>
    </div>
  )
}
