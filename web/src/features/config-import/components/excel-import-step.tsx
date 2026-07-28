/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Download, Loader2, Upload } from 'lucide-react'
import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ChannelLineScopeSelector } from '@/channel-config-converter/components/channel-line-scope-selector'
import {
  convertWorkbook,
  type WorkbookConversion,
} from '@/channel-config-converter/conversion'
import { serializeImportDocument } from '@/channel-config-converter/document'
import {
  buildScopedImportDocument,
  type ScopedImportDocumentResult,
} from '@/channel-config-converter/scope'
import { Button } from '@/components/ui/button'

import type { ConfigImportBatchDetail } from '../types'

export interface ExcelImportStepProps {
  disabled?: boolean
  convertFile?: (file: File) => Promise<WorkbookConversion>
  onUpload: (document: unknown) => Promise<ConfigImportBatchDetail>
  onUploaded: (batch: ConfigImportBatchDetail) => void
}

function downloadImportDocument(contents: string): void {
  const href = URL.createObjectURL(
    new Blob([contents], { type: 'application/json;charset=utf-8' })
  )
  const link = window.document.createElement('a')
  link.href = href
  link.download = 'channel-config-import.json'
  link.click()
  URL.revokeObjectURL(href)
}

export function ExcelImportStep(props: ExcelImportStepProps) {
  const { t } = useTranslation()
  const requestID = useRef(0)
  const [converted, setConverted] = useState<WorkbookConversion>()
  const [selectedLineRefs, setSelectedLineRefs] = useState<Set<string>>(
    new Set()
  )
  const [scoped, setScoped] = useState<ScopedImportDocumentResult>()
  const [isScopePending, setIsScopePending] = useState(false)
  const [isUploading, setIsUploading] = useState(false)
  const [error, setError] = useState<string>()

  useEffect(() => {
    if (!converted) {
      setScoped(undefined)
      setIsScopePending(false)
      return
    }

    let cancelled = false
    void buildScopedImportDocument(converted.document, selectedLineRefs)
      .then((nextScoped) => {
        if (!cancelled) {
          setScoped(nextScoped)
          setIsScopePending(false)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setIsScopePending(false)
          setError(t('The Excel file could not be converted.'))
        }
      })
    return () => {
      cancelled = true
    }
  }, [converted, selectedLineRefs, t])

  const handleFileChange = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file) return

    const currentRequestID = requestID.current + 1
    requestID.current = currentRequestID
    setConverted(undefined)
    setSelectedLineRefs(new Set())
    setScoped(undefined)
    setError(undefined)
    setIsScopePending(true)

    try {
      const result = await (props.convertFile ?? convertWorkbook)(file)
      if (currentRequestID !== requestID.current) return
      setConverted(result)
    } catch (caught) {
      if (currentRequestID !== requestID.current) return
      setIsScopePending(false)
      setError(
        caught instanceof Error && caught.message !== ''
          ? caught.message
          : t('The Excel file could not be converted.')
      )
    }
  }

  const handleSelectionChange = (lineRefs: Set<string>) => {
    setIsScopePending(true)
    setError(undefined)
    setSelectedLineRefs(lineRefs)
  }

  const handleImport = async () => {
    if (!scoped?.canUse || isScopePending || isUploading) return
    setIsUploading(true)
    setError(undefined)
    try {
      const batch = await props.onUpload(scoped.document)
      props.onUploaded(batch)
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : t('The import action failed.')
      )
    } finally {
      setIsUploading(false)
    }
  }

  const commandDisabled =
    props.disabled || isScopePending || isUploading || !scoped?.canUse

  return (
    <section className='space-y-4' aria-labelledby='config-import-excel-title'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <h2 id='config-import-excel-title' className='text-base font-semibold'>
          {t('Excel conversion')}
        </h2>
        <Button
          render={<label />}
          disabled={props.disabled || isScopePending || isUploading}
        >
          <Upload className='size-4' aria-hidden='true' />
          {t('Excel conversion')}
          <input
            accept='.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
            aria-label={t('Excel conversion')}
            className='sr-only'
            disabled={props.disabled || isScopePending || isUploading}
            onChange={handleFileChange}
            type='file'
          />
        </Button>
      </div>

      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      {scoped && (
        <ChannelLineScopeSelector
          disabled={props.disabled || isUploading}
          groups={scoped.groups}
          onSelectionChange={handleSelectionChange}
          selectedLineRefs={selectedLineRefs}
          summary={scoped}
        />
      )}

      {converted && (
        <div className='flex flex-wrap gap-2'>
          <Button
            disabled={commandDisabled}
            onClick={() => {
              if (!scoped) return
              downloadImportDocument(serializeImportDocument(scoped.document))
            }}
            type='button'
            variant='outline'
          >
            <Download className='size-4' aria-hidden='true' />
            {t('Export selected JSON')}
          </Button>
          <Button
            disabled={commandDisabled}
            onClick={() => void handleImport()}
            type='button'
          >
            {isUploading ? (
              <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            ) : (
              <Upload className='size-4' aria-hidden='true' />
            )}
            {t('Import selected configuration')}
          </Button>
        </div>
      )}
    </section>
  )
}
