import { api, type ApiRequestConfig } from '@/lib/api'

import { fetchTokenKey, getApiKeys } from '../keys/api'
import type { ApiKey, GetApiKeysParams, GetApiKeysResponse } from '../keys/types'
import { getUserModels } from '../playground/api'
import type { SeedanceVideoRequest, VideoTaskResponse } from './types'

export function getTokenRequestConfig(apiKey: string): ApiRequestConfig {
  return {
    authToken: apiKey,
    skipAuthRefresh: true,
    skipErrorHandler: true,
  }
}

type ApiKeyPageFetcher = (
  params: GetApiKeysParams
) => Promise<GetApiKeysResponse>

export async function loadVideoApiKeyPages(
  fetchPage: ApiKeyPageFetcher
): Promise<ApiKey[]> {
  const firstPage = await fetchPage({ p: 1, size: 100 })
  const firstData = firstPage.data
  if (!firstPage.success || !firstData) return []

  const pageSize = firstData.page_size || 100
  const pageCount = Math.ceil(firstData.total / pageSize)
  const remainingPages = await Promise.all(
    Array.from({ length: Math.max(0, pageCount - 1) }, (_, index) =>
      fetchPage({ p: index + 2, size: pageSize })
    )
  )
  const keys = [
    ...firstData.items,
    ...remainingPages.flatMap((response) => response.data?.items ?? []),
  ]

  return keys.filter((key) => key.status === 1)
}

export async function getVideoApiKeys(): Promise<ApiKey[]> {
  return loadVideoApiKeyPages(getApiKeys)
}

export async function getVideoModels(group: string): Promise<string[]> {
  if (!group) return []
  const models = await getUserModels(group)
  return models.map((model) => model.value)
}

export async function getVideoApiKeyValue(id: number): Promise<string> {
  const response = await fetchTokenKey(id)
  if (!response.success || !response.data?.key) {
    throw new Error(response.message || 'Unable to load API key')
  }
  return response.data.key
}

export async function createVideoTask(
  request: SeedanceVideoRequest,
  apiKey: string
): Promise<VideoTaskResponse> {
  const response = await api.post(
    '/api/v3/contents/generations/tasks',
    request,
    getTokenRequestConfig(apiKey)
  )
  return response.data as VideoTaskResponse
}

export async function queryVideoTask(
  taskId: string,
  apiKey: string
): Promise<VideoTaskResponse> {
  const response = await api.get(
    `/api/v3/contents/generations/tasks/${encodeURIComponent(taskId)}`,
    getTokenRequestConfig(apiKey)
  )
  return response.data as VideoTaskResponse
}
