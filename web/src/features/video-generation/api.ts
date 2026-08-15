import { api } from '@/lib/api'

import {
  getApiKeyRequestConfig,
  getApiKeyValue,
  getEnabledApiKeys,
} from '../keys/api'
import type { ApiKey } from '../keys/types'
import { getUserModels } from '../playground/api'
import type { SeedanceVideoRequest, VideoTaskResponse } from './types'

export {
  getApiKeyRequestConfig as getTokenRequestConfig,
  loadEnabledApiKeyPages as loadVideoApiKeyPages,
} from '../keys/api'

export async function getVideoApiKeys(): Promise<ApiKey[]> {
  return getEnabledApiKeys()
}

export async function getVideoModels(group: string): Promise<string[]> {
  if (!group) return []
  const models = await getUserModels(group)
  return models.map((model) => model.value)
}

export async function getVideoApiKeyValue(id: number): Promise<string> {
  return getApiKeyValue(id)
}

export async function createVideoTask(
  request: SeedanceVideoRequest,
  apiKey: string
): Promise<VideoTaskResponse> {
  const response = await api.post(
    '/api/v3/contents/generations/tasks',
    request,
    getApiKeyRequestConfig(apiKey)
  )
  return response.data as VideoTaskResponse
}

export async function queryVideoTask(
  taskId: string,
  apiKey: string
): Promise<VideoTaskResponse> {
  const response = await api.get(
    `/api/v3/contents/generations/tasks/${encodeURIComponent(taskId)}`,
    getApiKeyRequestConfig(apiKey)
  )
  return response.data as VideoTaskResponse
}
