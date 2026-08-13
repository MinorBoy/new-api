export type AssetStatus = 'pending' | 'processing' | 'active' | 'failed' | 'unknown'

export type Asset = {
  id: string
  type: string
  url: string
  status: AssetStatus
  provider: string
  provider_status?: string
  reference?: string
  created_at: number
  updated_at: number
}

export type AssetListResponse = {
  success: boolean
  data: { items: Asset[]; total: number; page: number; page_size: number }
}

export type AssetResponse = { success: boolean; data: Asset }
