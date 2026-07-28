/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { ConfigImportBatchDetail } from '../types'
import { ExcelImportStep } from './excel-import-step'
import { ImportUploadStep } from './import-upload-step'

export interface ImportSourceStepProps {
  disabled?: boolean
  onUpload: (document: unknown) => Promise<ConfigImportBatchDetail>
  onUploaded: (batch: ConfigImportBatchDetail) => void
}

export function ImportSourceStep(props: ImportSourceStepProps) {
  const { t } = useTranslation()
  const [isUploading, setIsUploading] = useState(false)
  const disabled = props.disabled || isUploading

  const handleUpload = async (document: unknown) => {
    setIsUploading(true)
    try {
      return await props.onUpload(document)
    } finally {
      setIsUploading(false)
    }
  }

  return (
    <Tabs
      className='min-h-0 flex-1 overflow-auto space-y-4 p-6'
      defaultValue='excel'
    >
      <TabsList variant='line'>
        <TabsTrigger disabled={disabled} value='excel'>
          {t('Excel conversion')}
        </TabsTrigger>
        <TabsTrigger disabled={disabled} value='json'>
          {t('JSON import')}
        </TabsTrigger>
      </TabsList>
      <TabsContent value='excel'>
        <ExcelImportStep
          disabled={disabled}
          onUpload={handleUpload}
          onUploaded={props.onUploaded}
        />
      </TabsContent>
      <TabsContent value='json'>
        <ImportUploadStep
          disabled={disabled}
          onUpload={handleUpload}
          onUploaded={props.onUploaded}
        />
      </TabsContent>
    </Tabs>
  )
}
