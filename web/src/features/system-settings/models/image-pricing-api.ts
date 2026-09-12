import { api } from '@/lib/api'

export type ImagePricingCostItem = {
  model: string
  sku: string
  known: boolean
  minimum_cost_usd?: string
  source_count: number
}

export type ImagePricingCostSummaryResponse = {
  success: boolean
  message: string
  data: {
    items: ImagePricingCostItem[]
  }
}

export async function getImagePricingCostSummary(
  models: string[]
): Promise<ImagePricingCostSummaryResponse> {
  const response = await api.get<ImagePricingCostSummaryResponse>(
    '/api/cost-accounting/image-pricing',
    { params: { model: models } }
  )
  if (!response.data.success) {
    throw new Error(
      response.data.message || 'Unable to load image pricing costs'
    )
  }
  return response.data
}
