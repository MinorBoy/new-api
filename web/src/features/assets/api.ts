import { api } from '@/lib/api'

import type { Asset, AssetListResponse, AssetResponse } from './types'

export type AssetListParams = { page?: number; pageSize?: number }

export async function listAssets(
  params: AssetListParams = {}
): Promise<AssetListResponse> {
  const response = await api.get<AssetListResponse>('/api/v3/assets', {
    params: {
      type: 'image',
      page: params.page ?? 1,
      page_size: params.pageSize ?? 20,
    },
  })
  return response.data
}

export async function createAsset(url: string): Promise<AssetResponse> {
  const response = await api.post<AssetResponse>(
    '/api/v3/assets',
    { type: 'image', url },
    { headers: { 'Idempotency-Key': crypto.randomUUID() } }
  )
  return response.data
}

export async function refreshAsset(asset: Asset): Promise<AssetResponse> {
  const response = await api.post<AssetResponse>(
    `/api/v3/assets/${asset.id}/refresh`
  )
  return response.data
}
