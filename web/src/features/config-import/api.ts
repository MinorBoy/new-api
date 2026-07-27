/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { api } from '@/lib/api'

import {
  configImportBatchDetailResponseSchema,
  configImportBatchPageResponseSchema,
  configImportListParamsSchema,
  configImportPublishResponseSchema,
  type ConfigImportBatchDetail,
  type ConfigImportBatchPage,
  type ConfigImportBindingsRequest,
  type ConfigImportListParams,
  type ConfigImportResolutionsRequest,
  type ConfigImportStageRequest,
} from './types'

const CONFIG_IMPORT_PATH = '/api/config-import/batches'

function parseDetail(responseData: unknown): ConfigImportBatchDetail {
  return configImportBatchDetailResponseSchema.parse(responseData).data
}

export async function uploadConfigImport(
  document: unknown
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}`, { document })
  return parseDetail(response.data)
}

export async function listConfigImportBatches(
  params: ConfigImportListParams = {}
): Promise<ConfigImportBatchPage> {
  const parsedParams = configImportListParamsSchema.parse(params)
  const response = await api.get(CONFIG_IMPORT_PATH, { params: parsedParams })
  return configImportBatchPageResponseSchema.parse(response.data).data
}

export async function getConfigImportBatch(
  id: number
): Promise<ConfigImportBatchDetail> {
  const response = await api.get(`${CONFIG_IMPORT_PATH}/${id}`)
  return parseDetail(response.data)
}

export async function saveConfigImportBindings(
  id: number,
  request: ConfigImportBindingsRequest
): Promise<ConfigImportBatchDetail> {
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/bindings`,
    request
  )
  return parseDetail(response.data)
}

export async function saveConfigImportResolutions(
  id: number,
  request: ConfigImportResolutionsRequest
): Promise<ConfigImportBatchDetail> {
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/resolutions`,
    request
  )
  return parseDetail(response.data)
}

export async function stageConfigImport(
  id: number,
  request: ConfigImportStageRequest = {}
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/stage`, request)
  return parseDetail(response.data)
}

export async function validateConfigImport(
  id: number
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/validate`)
  return parseDetail(response.data)
}

export async function publishConfigImport(
  id: number
): Promise<{ batch_id: number; status: 'published' }> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/publish`)
  return configImportPublishResponseSchema.parse(response.data).data
}
