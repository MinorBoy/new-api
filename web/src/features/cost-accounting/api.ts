/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import { filenameFromContentDisposition } from './lib/catalog-export'
import type {
  CostAccountingApiResponse,
  CostAccountingSettings,
  CostAnomalyPage,
  CostAnomalyParams,
  CostCatalogDetail,
  CostCatalogExportResult,
  CostCatalogPage,
  CostCatalogParams,
  CostCoverageItem,
  CostCoverageParams,
  CostPreviewRequest,
  CostPreviewResult,
  CostProfitBreakdown,
  CostProfitSummary,
  CostReportParams,
  CostRequestDetail,
  CostRule,
  CostRuleListParams,
  CostRuleUpdateRequest,
  CostRuleValidationResult,
  CostRuleWriteRequest,
  ReconcileCostAttemptRequest,
  ReconcileCostRevenueRequest,
  UpdateCostAccountingSettingsRequest,
} from './types'

const COST_ACCOUNTING_PATH = '/api/cost-accounting'

export const costAccountingQueryKeys = {
  all: ['cost-accounting'] as const,
  settings: () => [...costAccountingQueryKeys.all, 'settings'] as const,
  rules: () => [...costAccountingQueryKeys.all, 'rules'] as const,
  ruleList: (params: CostRuleListParams = {}) =>
    [...costAccountingQueryKeys.rules(), params] as const,
  ruleHistory: (id: number) =>
    [...costAccountingQueryKeys.rules(), id, 'history'] as const,
  coverages: () => [...costAccountingQueryKeys.all, 'coverage'] as const,
  coverage: (params: CostCoverageParams = {}) =>
    [...costAccountingQueryKeys.coverages(), params] as const,
  requests: () => [...costAccountingQueryKeys.all, 'requests'] as const,
  request: (id: number) => [...costAccountingQueryKeys.requests(), id] as const,
  anomalies: () => [...costAccountingQueryKeys.all, 'anomalies'] as const,
  anomalyList: (params: CostAnomalyParams = {}) =>
    [...costAccountingQueryKeys.anomalies(), params] as const,
  catalogs: () => [...costAccountingQueryKeys.all, 'catalog'] as const,
  catalog: (params: CostCatalogParams) =>
    [...costAccountingQueryKeys.catalogs(), params] as const,
  catalogDetail: (id: number) =>
    [...costAccountingQueryKeys.catalogs(), id] as const,
  reports: () => [...costAccountingQueryKeys.all, 'reports'] as const,
  reportSummary: (params: CostReportParams = {}) =>
    [...costAccountingQueryKeys.reports(), 'summary', params] as const,
  reportBreakdown: (params: CostReportParams = {}) =>
    [...costAccountingQueryKeys.reports(), 'breakdown', params] as const,
}

export async function getCostAccountingSettings(): Promise<
  CostAccountingApiResponse<CostAccountingSettings>
> {
  const response = await api.get<
    CostAccountingApiResponse<CostAccountingSettings>
  >(`${COST_ACCOUNTING_PATH}/settings`)
  return response.data
}

export async function updateCostAccountingSettings(
  request: UpdateCostAccountingSettingsRequest
): Promise<CostAccountingApiResponse<CostAccountingSettings>> {
  const response = await api.put<
    CostAccountingApiResponse<CostAccountingSettings>
  >(`${COST_ACCOUNTING_PATH}/settings`, request)
  return response.data
}

export async function listCostRules(
  params: CostRuleListParams = {}
): Promise<CostAccountingApiResponse<CostRule[]>> {
  const response = await api.get<CostAccountingApiResponse<CostRule[]>>(
    `${COST_ACCOUNTING_PATH}/rules`,
    { params }
  )
  return response.data
}

export async function createCostRule(
  request: CostRuleWriteRequest
): Promise<CostAccountingApiResponse<CostRule>> {
  const response = await api.post<CostAccountingApiResponse<CostRule>>(
    `${COST_ACCOUNTING_PATH}/rules`,
    request
  )
  return response.data
}

export async function updateCostRule(
  id: number,
  request: CostRuleUpdateRequest
): Promise<CostAccountingApiResponse<CostRule>> {
  const response = await api.put<CostAccountingApiResponse<CostRule>>(
    `${COST_ACCOUNTING_PATH}/rules/${id}`,
    request
  )
  return response.data
}

export async function validateCostRule(
  id: number
): Promise<CostAccountingApiResponse<CostRuleValidationResult>> {
  const response = await api.post<
    CostAccountingApiResponse<CostRuleValidationResult>
  >(`${COST_ACCOUNTING_PATH}/rules/${id}/validate`)
  return response.data
}

export async function activateCostRule(
  id: number
): Promise<CostAccountingApiResponse<CostRule>> {
  const response = await api.post<CostAccountingApiResponse<CostRule>>(
    `${COST_ACCOUNTING_PATH}/rules/${id}/activate`
  )
  return response.data
}

export async function retireCostRule(
  id: number
): Promise<CostAccountingApiResponse<null>> {
  const response = await api.post<CostAccountingApiResponse<null>>(
    `${COST_ACCOUNTING_PATH}/rules/${id}/retire`
  )
  return response.data
}

export async function getCostRuleHistory(
  id: number
): Promise<CostAccountingApiResponse<CostRule[]>> {
  const response = await api.get<CostAccountingApiResponse<CostRule[]>>(
    `${COST_ACCOUNTING_PATH}/rules/${id}/history`
  )
  return response.data
}

export async function previewCostAccounting(
  request: CostPreviewRequest
): Promise<CostAccountingApiResponse<CostPreviewResult>> {
  const response = await api.post<CostAccountingApiResponse<CostPreviewResult>>(
    `${COST_ACCOUNTING_PATH}/preview`,
    request
  )
  return response.data
}

export async function getCostCoverage(
  params: CostCoverageParams = {}
): Promise<CostAccountingApiResponse<CostCoverageItem[]>> {
  const response = await api.get<CostAccountingApiResponse<CostCoverageItem[]>>(
    `${COST_ACCOUNTING_PATH}/coverage`,
    { params }
  )
  return response.data
}

export async function getCostAccountingRequest(
  id: number
): Promise<CostAccountingApiResponse<CostRequestDetail>> {
  const response = await api.get<CostAccountingApiResponse<CostRequestDetail>>(
    `${COST_ACCOUNTING_PATH}/requests/${id}`
  )
  return response.data
}

export async function listCostAnomalies(
  params: CostAnomalyParams = {}
): Promise<CostAccountingApiResponse<CostAnomalyPage>> {
  const response = await api.get<CostAccountingApiResponse<CostAnomalyPage>>(
    `${COST_ACCOUNTING_PATH}/anomalies`,
    { params }
  )
  return response.data
}

export async function reconcileCostAttempt(
  id: number,
  request: ReconcileCostAttemptRequest
): Promise<CostAccountingApiResponse<null>> {
  const response = await api.post<CostAccountingApiResponse<null>>(
    `${COST_ACCOUNTING_PATH}/attempts/${id}/reconcile`,
    request
  )
  return response.data
}

export async function reconcileCostRevenue(
  id: number,
  request: ReconcileCostRevenueRequest
): Promise<CostAccountingApiResponse<null>> {
  const response = await api.post<CostAccountingApiResponse<null>>(
    `${COST_ACCOUNTING_PATH}/requests/${id}/reconcile-revenue`,
    request
  )
  return response.data
}

export async function getCostReportSummary(
  params: CostReportParams = {}
): Promise<CostAccountingApiResponse<CostProfitSummary>> {
  const response = await api.get<CostAccountingApiResponse<CostProfitSummary>>(
    `${COST_ACCOUNTING_PATH}/reports/summary`,
    { params }
  )
  return response.data
}

export async function getCostReportBreakdown(
  params: CostReportParams = {}
): Promise<CostAccountingApiResponse<CostProfitBreakdown[]>> {
  const response = await api.get<
    CostAccountingApiResponse<CostProfitBreakdown[]>
  >(`${COST_ACCOUNTING_PATH}/reports/breakdown`, { params })
  return response.data
}

export async function getSupplierCostCatalog(
  params: CostCatalogParams
): Promise<CostAccountingApiResponse<CostCatalogPage>> {
  const response = await api.get<CostAccountingApiResponse<CostCatalogPage>>(
    `${COST_ACCOUNTING_PATH}/catalog`,
    { params }
  )
  return response.data
}

export async function getSupplierCostCatalogDetail(
  id: number
): Promise<CostAccountingApiResponse<CostCatalogDetail>> {
  const response = await api.get<CostAccountingApiResponse<CostCatalogDetail>>(
    `${COST_ACCOUNTING_PATH}/catalog/${id}`
  )
  return response.data
}

export async function exportSupplierCostCatalog(
  scope: 'filtered' | 'all',
  params: Omit<CostCatalogParams, 'page' | 'page_size'>
): Promise<CostCatalogExportResult> {
  const response = await api.get<Blob>(
    `${COST_ACCOUNTING_PATH}/catalog/export`,
    { params: { ...params, scope }, responseType: 'blob' }
  )
  const disposition = response.headers['content-disposition']
  return {
    blob: response.data,
    filename: filenameFromContentDisposition(
      typeof disposition === 'string' ? disposition : undefined,
      'supplier-cost-catalog.csv'
    ),
    rowCount: validExportRowCount(response.headers['x-exported-row-count']),
  }
}

function validExportRowCount(value: unknown): number {
  const count = Number(value)
  return Number.isSafeInteger(count) && count >= 0 ? count : 0
}
