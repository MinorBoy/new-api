import { api } from '@/lib/api'

import { getApiKeyRequestConfig } from '../keys/api'
import type { Asset, AssetListResponse, AssetResponse } from './types'

export type AssetListParams = { page?: number; pageSize?: number }

export async function listAssets(
  apiKey: string,
  params: AssetListParams = {}
): Promise<AssetListResponse> {
  const response = await api.get<AssetListResponse>('/api/v3/assets', {
    ...getApiKeyRequestConfig(apiKey),
    params: {
      type: 'image',
      page: params.page ?? 1,
      page_size: params.pageSize ?? 20,
    },
  })
  return response.data
}

export async function createAsset(
  apiKey: string,
  url: string
): Promise<AssetResponse> {
  const response = await api.post<AssetResponse>(
    '/api/v3/assets',
    { type: 'image', url },
    {
      ...getApiKeyRequestConfig(apiKey),
      headers: { 'Idempotency-Key': crypto.randomUUID() },
    }
  )
  return response.data
}

export async function refreshAsset(
  apiKey: string,
  asset: Asset
): Promise<AssetResponse> {
  const response = await api.post<AssetResponse>(
    `/api/v3/assets/${asset.id}/refresh`,
    undefined,
    getApiKeyRequestConfig(apiKey)
  )
  return response.data
}
