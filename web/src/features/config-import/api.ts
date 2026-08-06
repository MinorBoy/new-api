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
  configImportBindingsRequestSchema,
  configImportListParamsSchema,
  configImportPublishResponseSchema,
  configImportPricingReviewRequestSchema,
  configImportResolutionsRequestSchema,
  configImportRouteReviewsRequestSchema,
  configImportStageRequestSchema,
  configImportUploadRequestSchema,
  type ConfigImportBatchDetail,
  type ConfigImportBatchPage,
  type ConfigImportBindingsRequest,
  type ConfigImportListParams,
  type ConfigImportPricingReviewRequest,
  type ConfigImportResolutionsRequest,
  type ConfigImportRouteReviewsRequest,
  type ConfigImportStageRequest,
} from './types'

const CONFIG_IMPORT_PATH = '/api/config-import/batches'

function parseDetail(responseData: unknown): ConfigImportBatchDetail {
  return configImportBatchDetailResponseSchema.parse(responseData).data
}

export async function uploadConfigImport(
  document: unknown
): Promise<ConfigImportBatchDetail> {
  const request = configImportUploadRequestSchema.parse({ document })
  const response = await api.post(`${CONFIG_IMPORT_PATH}`, request)
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

export async function copyConfigImportBatchForBinding(
  id: number
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(
    `${CONFIG_IMPORT_PATH}/${id}/copy-for-binding`
  )
  return parseDetail(response.data)
}

export async function saveConfigImportBindings(
  id: number,
  request: ConfigImportBindingsRequest
): Promise<ConfigImportBatchDetail> {
  const payload = configImportBindingsRequestSchema.parse(request)
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/bindings`,
    payload
  )
  return parseDetail(response.data)
}

export async function saveConfigImportResolutions(
  id: number,
  request: ConfigImportResolutionsRequest
): Promise<ConfigImportBatchDetail> {
  const payload = configImportResolutionsRequestSchema.parse(request)
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/resolutions`,
    payload
  )
  return parseDetail(response.data)
}

export async function saveConfigImportRouteReviews(
  id: number,
  request: ConfigImportRouteReviewsRequest
): Promise<ConfigImportBatchDetail> {
  const payload = configImportRouteReviewsRequestSchema.parse(request)
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/route-reviews`,
    payload
  )
  return parseDetail(response.data)
}

export async function saveConfigImportPricingReview(
  id: number,
  request: ConfigImportPricingReviewRequest
): Promise<ConfigImportBatchDetail> {
  const payload = configImportPricingReviewRequestSchema.parse(request)
  const response = await api.put(
    `${CONFIG_IMPORT_PATH}/${id}/pricing-review`,
    payload
  )
  return parseDetail(response.data)
}

export async function stageConfigImport(
  id: number,
  request: ConfigImportStageRequest = {}
): Promise<ConfigImportBatchDetail> {
  const payload = configImportStageRequestSchema.parse(request)
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/stage`, payload)
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

export async function activateConfigImport(
  id: number
): Promise<ConfigImportBatchDetail> {
  const response = await api.post(`${CONFIG_IMPORT_PATH}/${id}/activate`)
  return parseDetail(response.data)
}

export async function refreshConfigImportCache(
  id: number
): Promise<ConfigImportBatchDetail> {
  await api.post(`${CONFIG_IMPORT_PATH}/${id}/refresh-cache`)
  return getConfigImportBatch(id)
}
