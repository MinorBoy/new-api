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
import Decimal from 'decimal.js'

import { cellText, type SourceRecord, type SourceWorkbook } from './source'
import type {
  ChannelRow,
  CostRow,
  Issue,
  MappingRow,
  ModelRule,
  ProfitRow,
  Rules,
  SaleRow,
  SkuRow,
  TemplateData,
} from './types'
import { validateTemplateData } from './validate'

const COST_MODES = {
  second: 'per_duration',
  call: 'per_request',
  token: 'per_token',
} as const

type CostMode = (typeof COST_MODES)[keyof typeof COST_MODES]

function field(record: SourceRecord | undefined, name: string): string {
  if (!record) return ''
  return cellText(record.fields[name])
}

function numericField(
  record: SourceRecord | undefined,
  name: string
): Decimal | null {
  if (!record) return null
  const text = field(record, name).replace(/[%￥¥,]/gu, '')
  if (text === '') return null
  try {
    const value = new Decimal(text)
    return value.isFinite() ? value : null
  } catch {
    return null
  }
}

function slug(value: string): string {
  return value
    .trim()
    .toUpperCase()
    .replace(/[^A-Z0-9]+/gu, '-')
    .replace(/^-+|-+$/gu, '')
}

function resolution(value: string): string {
  const normalized = value.trim().toLowerCase()
  if (normalized === '') return 'default'
  return normalized.endsWith('p') || normalized === '4k'
    ? normalized
    : `${normalized}p`
}

function parseDuration(value: string): [number, number] {
  const match = value.match(/(\d+(?:\.\d+)?)\s*[-~至]\s*(\d+(?:\.\d+)?)/u)
  if (!match) return [0, 0]
  return [Number(match[1]), Number(match[2])]
}

function booleanLabel(
  value: string,
  fallback: '是' | '否' | '待确认'
): '是' | '否' | '待确认' {
  if (['是', 'yes', 'true', '1'].includes(value.toLowerCase())) return '是'
  if (['否', 'no', 'false', '0'].includes(value.toLowerCase())) return '否'
  return fallback
}

function scenarioCode(scenario: 'no_video' | 'with_video'): string {
  return scenario === 'no_video' ? 'NOV' : 'VID'
}

function issue(
  code: string,
  severity: Issue['severity'],
  message: string,
  record: SourceRecord,
  businessId?: string
): Issue {
  return {
    code,
    severity,
    message,
    businessId,
    sheet: record.location.sheet,
    row: record.location.row,
  }
}

function decimalString(value: Decimal | null, fallback = '0'): string {
  return value === null ? fallback : value.toFixed()
}

function modelRuleFor(
  rawModel: string,
  rules: Rules,
  officialModel: string
): ModelRule {
  return (
    rules.modelRules[rawModel] ?? {
      clientModel: officialModel || rawModel,
      upstreamModel: rawModel,
    }
  )
}

function sourceIdForChannel(channel: SourceRecord): string {
  return `SRC-CH-${slug(field(channel, '渠道'))}`
}

function buildChannels(source: SourceWorkbook, rules: Rules): ChannelRow[] {
  return source.channels.map((channel) => {
    const sourceCode = field(channel, '渠道')
    const businessId =
      rules.channelCodes[sourceCode] ?? `CH-RAW-${slug(sourceCode)}`
    const model = source.models.find(
      (candidate) => field(candidate, '渠道') === sourceCode
    )
    const status = model && field(model, '状态') === '正常' ? 'active' : 'draft'
    return {
      businessId,
      name: field(channel, '名称'),
      pricePage: field(channel, '链接'),
      currency: rules.defaults.currency,
      rechargeRatio: rules.defaults.rechargeRatio,
      feeRate: decimalString(model ? numericField(model, '手续费') : null),
      billingMultiplier: decimalString(
        model ? numericField(model, '计费倍率') : null,
        '1'
      ),
      status,
      strictCostValidation: '是',
      sourceId: sourceIdForChannel(channel),
      sourceSheet: channel.location.sheet,
      sourceRow: channel.location.row,
      note: `Base Url=${field(channel, 'Base Url')}`,
    }
  })
}

type OfficialIndex = Map<string, SourceRecord[]>

function indexOfficialPrices(source: SourceWorkbook): OfficialIndex {
  const index: OfficialIndex = new Map()
  for (const price of source.officialPrices) {
    const key = `${field(price, '模型')}\u0000${resolution(field(price, '分辨率'))}`
    const values = index.get(key) ?? []
    values.push(price)
    index.set(key, values)
  }
  return index
}

function findOfficial(
  index: OfficialIndex,
  model: string,
  resolutionValue: string
): SourceRecord | undefined {
  return index.get(`${model}\u0000${resolution(resolutionValue)}`)?.[0]
}

function skuId(
  model: string,
  version: string,
  resolutionValue: string
): string {
  return `SKU-${slug(model)}-${slug(version || 'standard')}-${slug(resolutionValue)}`
}

function buildSkus(
  source: SourceWorkbook,
  rules: Rules,
  officialIndex: OfficialIndex,
  issues: Issue[]
): SkuRow[] {
  const rows = new Map<string, SkuRow>()
  for (const modelRecord of source.models) {
    const rawModel = field(modelRecord, '模型ID')
    const official = source.officialPrices.find(
      (price) => field(price, '模型') === rawModel
    )
    const modelRule = modelRuleFor(rawModel, rules, field(official, '模型'))
    const model = modelRule.clientModel ?? field(official, '模型') ?? rawModel
    const resolutionValue = resolution(field(modelRecord, '清晰度'))
    const officialPrice = findOfficial(officialIndex, model, resolutionValue)
    if (!officialPrice) continue
    const version =
      field(officialPrice, '版本') || modelRule.clientModel || '标准'
    const [minDuration, maxDuration] = parseDuration(
      field(modelRecord, '时长范围')
    )
    const [ruleMin, ruleMax] = [
      modelRule.minDurationSeconds ?? minDuration,
      modelRule.maxDurationSeconds ?? maxDuration,
    ]
    const id = skuId(model, version, resolutionValue)
    if (rows.has(id)) continue
    const width = modelRule.outputWidth ?? Number(field(officialPrice, '长边'))
    const height =
      modelRule.outputHeight ?? Number(field(officialPrice, '短边'))
    const frameRate =
      modelRule.frameRate ?? Number(field(officialPrice, '帧率'))
    const sourceId = `SRC-OFFICIAL-${slug(model)}`
    rows.set(id, {
      businessId: id,
      model,
      version,
      resolution: resolutionValue,
      outputWidth: width,
      outputHeight: height,
      frameRate,
      minDurationSeconds: ruleMin,
      maxDurationSeconds: ruleMax,
      ratio: field(modelRecord, '比例') || '按渠道映射',
      supportsVideoInput: booleanLabel(field(modelRecord, '视频输入'), '是'),
      supportsRealPerson:
        modelRule.supportsRealPerson === undefined
          ? '待确认'
          : modelRule.supportsRealPerson
            ? '是'
            : '否',
      supportsSuperResolution:
        modelRule.supportsSuperResolution === undefined
          ? '待确认'
          : modelRule.supportsSuperResolution
            ? '是'
            : '否',
      measurementMethod: 'video_pixel_tokens',
      status: 'active',
      sourceId,
      sourceSheet: officialPrice.location.sheet,
      sourceRow: officialPrice.location.row,
      note: '由官方价格矩阵和源模型能力合并生成。',
    })
  }
  if (rows.size === 0 && source.models.length > 0) {
    issues.push(
      issue(
        'SKU_UNRESOLVED',
        'WARN',
        'No official SKU matched the source models.',
        source.models[0]
      )
    )
  }
  return [...rows.values()].sort((left, right) =>
    left.businessId.localeCompare(right.businessId)
  )
}

function buildSales(
  skus: SkuRow[],
  officialIndex: OfficialIndex,
  rules: Rules
): SaleRow[] {
  const rows: SaleRow[] = []
  for (const sku of skus) {
    const official = findOfficial(officialIndex, sku.model, sku.resolution)
    if (!official) continue
    const sourceId = `SRC-OFFICIAL-${slug(sku.model)}`
    for (const scenario of ['no_video', 'with_video'] as const) {
      const nativePerMillion = decimalString(
        numericField(
          official,
          scenario === 'no_video' ? '不含视频 元/M' : '包含视频 元/M'
        )
      )
      const nativePerSecond = decimalString(
        numericField(
          official,
          scenario === 'no_video' ? '不含视频 元/秒' : '包含视频 元/秒'
        )
      )
      const currencyToUsd = new Decimal(rules.defaults.currencyToUsd)
      rows.push({
        businessId: `SALE-${slug(sku.model)}-${slug(sku.resolution)}-${scenarioCode(scenario)}`,
        clientModel: sku.model,
        skuCode: sku.businessId,
        scenario,
        billingMode: 'per_token',
        currency: rules.defaults.currency,
        nativePerMillion,
        outputWidth: sku.outputWidth,
        outputHeight: sku.outputHeight,
        frameRate: sku.frameRate,
        nativePerSecond,
        usdPerMillion: new Decimal(nativePerMillion)
          .mul(currencyToUsd)
          .toFixed(),
        usdPerSecond: new Decimal(nativePerSecond).mul(currencyToUsd).toFixed(),
        status: 'active',
        sourceId,
        sourceSheet: official.location.sheet,
        sourceRow: official.location.row,
        note: '由 sd官价生成；USD 为预览值。',
      })
    }
  }
  return rows.sort((left, right) =>
    left.businessId.localeCompare(right.businessId)
  )
}

function priceForMode(record: SourceRecord, mode: CostMode): Decimal | null {
  if (mode === 'per_duration') return numericField(record, '元/秒')
  if (mode === 'per_request') return numericField(record, '元/次')
  return numericField(record, '元/1M')
}

function buildCostsAndMappings(
  source: SourceWorkbook,
  rules: Rules,
  channels: ChannelRow[],
  skus: SkuRow[],
  officialIndex: OfficialIndex,
  issues: Issue[]
): { costs: CostRow[]; mappings: MappingRow[] } {
  const skuByModelResolution = new Map(
    skus.map((sku) => [`${sku.model}\u0000${sku.resolution}`, sku])
  )
  const costs: CostRow[] = []
  const mappings: MappingRow[] = []
  for (const record of source.models) {
    const rawModel = field(record, '模型ID')
    const sourceChannel = field(record, '渠道')
    const channelCode =
      rules.channelCodes[sourceChannel] ?? `CH-RAW-${slug(sourceChannel)}`
    const modelRule = modelRuleFor(rawModel, rules, '')
    const clientModel = modelRule.clientModel ?? rawModel
    const upstreamModel = modelRule.upstreamModel ?? rawModel
    const resolutionValue = resolution(field(record, '清晰度'))
    const resolutionId = slug(resolutionValue).replace(/P$/u, '')
    const sku = skuByModelResolution.get(
      `${clientModel}\u0000${resolutionValue}`
    )
    const skuCode =
      sku?.businessId ??
      skuId(clientModel, field(record, '版本'), resolutionValue)
    const modeName = field(record, '计费').toLowerCase()
    const mode = COST_MODES[modeName as keyof typeof COST_MODES]
    const baseId = `R${record.location.row}`
    const channelSource = source.channels.find(
      (candidate) => field(candidate, '渠道') === sourceChannel
    )
    const sourceId = channelSource
      ? sourceIdForChannel(channelSource)
      : `SRC-${slug(sourceChannel)}-${baseId}`
    const override =
      rules.overrides[`${sourceChannel}/${baseId}`] ??
      rules.overrides[String(record.location.row)]
    const effectiveMode = mode
    if (!effectiveMode) {
      issues.push(
        issue(
          'COST_MODE_UNKNOWN',
          'FAIL',
          `Unsupported billing mode: ${modeName}`,
          record
        )
      )
      continue
    }
    const native = priceForMode(record, effectiveMode)
    if (native === null || native.lte(0)) {
      issues.push(
        issue(
          'COST_PRICE_INVALID',
          'FAIL',
          `Missing positive source price for ${modeName}.`,
          record
        )
      )
    }
    if (!sku) {
      issues.push(
        issue(
          'SKU_UNRESOLVED',
          'WARN',
          `No official SKU matched ${clientModel}/${resolutionValue}.`,
          record
        )
      )
    }
    const [minDuration, maxDuration] = parseDuration(field(record, '时长范围'))
    const billingMultiplier = numericField(record, '计费倍率') ?? new Decimal(1)
    const feeRate = numericField(record, '手续费') ?? new Decimal(0)
    const currencyToUsd = new Decimal(rules.defaults.currencyToUsd)
    const purchaseDiscount = new Decimal(rules.defaults.purchaseDiscountRatio)
    const rechargeRatio = new Decimal(rules.defaults.rechargeRatio)
    const normalized = (native ?? new Decimal(0))
      .mul(billingMultiplier)
      .mul(purchaseDiscount)
      .div(rechargeRatio)
      .mul(new Decimal(1).add(feeRate))
      .mul(currencyToUsd)
    const status: 'active' | 'draft' =
      sku && field(record, '状态') === '正常' ? 'active' : 'draft'
    if (override?.status) {
      // An explicit rule decision is the only allowed status override.
      // It is still subject to unresolved-reference validation below.
    }
    const costStatus = override?.status ?? status
    for (const scenario of ['no_video', 'with_video'] as const) {
      const suffix = scenarioCode(scenario)
      costs.push({
        businessId: `COST-${slug(channelCode.replace(/^CH-/, ''))}-${baseId}-${resolutionId}-${effectiveMode === 'per_duration' ? 'DUR' : effectiveMode === 'per_request' ? 'REQ' : 'TOK'}-${suffix}`,
        channelCode,
        upstreamModel,
        skuCode,
        scenario,
        mode: effectiveMode,
        tokenSubMode: effectiveMode === 'per_token' ? 'total_tokens' : '',
        meterSource: effectiveMode === 'per_token' ? 'upstream_usage' : '',
        tokenField: effectiveMode === 'per_token' ? 'total_tokens' : '',
        chargeEvent:
          effectiveMode === 'per_request'
            ? 'response_succeeded'
            : 'response_succeeded',
        currency: rules.defaults.currency,
        nativePerRequest:
          effectiveMode === 'per_request' ? decimalString(native) : '',
        nativePerSecond:
          effectiveMode === 'per_duration' ? decimalString(native) : '',
        nativePerMillion:
          effectiveMode === 'per_token' ? decimalString(native) : '',
        nativeBasePrice: decimalString(native),
        billingMultiplier: billingMultiplier.toFixed(),
        purchaseDiscountRatio: purchaseDiscount.toFixed(),
        rechargeRatio: rechargeRatio.toFixed(),
        feeRate: feeRate.toFixed(),
        currencyToUsd: currencyToUsd.toFixed(),
        normalizedUsdUnitPrice: normalized.toFixed(),
        unit:
          effectiveMode === 'per_duration'
            ? 'USD/second'
            : effectiveMode === 'per_request'
              ? 'USD/request'
              : 'USD/1M tokens',
        status: costStatus,
        sourceId,
        sourceSheet: record.location.sheet,
        sourceRow: record.location.row,
        note: `时长=${minDuration}-${maxDuration}; 原模型=${rawModel}; 原备注=${field(record, '备注')}`,
      })
    }
    mappings.push({
      businessId: `MAP-${slug(channelCode.replace(/^CH-/, ''))}-${baseId}-${resolutionId}`,
      clientModel,
      channelCode,
      upstreamModel,
      skuCode,
      defaultScenario: 'no_video',
      enabled: costStatus === 'active' ? '是' : '否',
      sourceId,
      sourceSheet: record.location.sheet,
      sourceRow: record.location.row,
      note: `原模型=${rawModel}; 原比例=${field(record, '比例')}; 真人脸=${field(record, '过真人脸')}`,
    })
    if (!channels.some((channel) => channel.businessId === channelCode)) {
      issues.push(
        issue(
          'CHANNEL_UNRESOLVED',
          'FAIL',
          `Unknown channel code ${channelCode}.`,
          record
        )
      )
    }
    if (!officialIndex.has(`${clientModel}\u0000${resolutionValue}`)) {
      issues.push(
        issue(
          'SALE_UNRESOLVED',
          'WARN',
          `No official price matched ${clientModel}/${resolutionValue}.`,
          record
        )
      )
    }
  }
  costs.sort((left, right) => left.businessId.localeCompare(right.businessId))
  mappings.sort((left, right) =>
    left.businessId.localeCompare(right.businessId)
  )
  return { costs, mappings }
}

function buildProfits(
  sales: SaleRow[],
  costs: CostRow[],
  skus: SkuRow[],
  groupRatio: string
): ProfitRow[] {
  const salesByKey = new Map(
    sales.map((sale) => [`${sale.skuCode}\u0000${sale.scenario}`, sale])
  )
  const skuById = new Map(skus.map((sku) => [sku.businessId, sku]))
  return costs.map((cost) => {
    const sale = salesByKey.get(`${cost.skuCode}\u0000${cost.scenario}`)
    const sku = skuById.get(cost.skuCode)
    const inputSeconds = sku?.minDurationSeconds ?? 0
    const outputSeconds = sku?.minDurationSeconds ?? 0
    const tokenCount = Math.floor(
      ((inputSeconds + outputSeconds) *
        (sku?.outputWidth ?? 0) *
        (sku?.outputHeight ?? 0) *
        (sku?.frameRate ?? 0)) /
        1024
    )
    const saleUsd = new Decimal(sale?.usdPerMillion ?? sale?.usdPerSecond ?? 0)
    const revenue = saleUsd.mul(groupRatio)
    const channelCost =
      cost.mode === 'per_token'
        ? new Decimal(cost.normalizedUsdUnitPrice)
            .mul(tokenCount)
            .div(1_000_000)
        : new Decimal(cost.normalizedUsdUnitPrice)
    const profit = revenue.sub(channelCost)
    return {
      businessId: `PROFIT-${cost.businessId}`,
      saleId: sale?.businessId ?? '',
      costId: cost.businessId,
      groupRatio,
      inputVideoSeconds: inputSeconds,
      outputVideoSeconds: outputSeconds,
      skuCode: cost.skuCode,
      scenario: cost.scenario,
      estimatedTokens: tokenCount,
      costMode: cost.mode,
      normalizedUsdUnitPrice: cost.normalizedUsdUnitPrice,
      officialSaleUsd: saleUsd.toFixed(),
      channelCostUsd: channelCost.toFixed(),
      userRevenueUsd: revenue.toFixed(),
      grossProfitUsd: profit.toFixed(),
      grossMargin: revenue.isZero() ? '' : profit.div(revenue).toFixed(),
      costStatus: cost.status,
      note: sale
        ? '利润为配置预览，不是结算账本。'
        : '缺少官方售价，利润仅保留草稿。',
    }
  })
}

export function buildTemplateData(
  source: SourceWorkbook,
  rules: Rules
): TemplateData {
  const issues: Issue[] = []
  const officialIndex = indexOfficialPrices(source)
  const channels = buildChannels(source, rules)
  const skus = buildSkus(source, rules, officialIndex, issues)
  const sales = buildSales(skus, officialIndex, rules)
  const { costs, mappings } = buildCostsAndMappings(
    source,
    rules,
    channels,
    skus,
    officialIndex,
    issues
  )
  const profits = buildProfits(sales, costs, skus, rules.defaults.groupRatio)
  const sources = [
    ...source.channels.map((channel) => ({
      businessId: sourceIdForChannel(channel),
      project: field(channel, '名称'),
      valueOrRange: field(channel, '链接'),
      unit: '',
      asOf: '',
      sourceType: 'workbook',
      sourceName: 'sd收录.xlsx',
      reference: `${channel.location.sheet}!${channel.location.row}`,
      owner: '',
      note: field(channel, 'Base Url'),
      accessedAt: new Date().toISOString().slice(0, 10),
    })),
    ...[
      ...new Map(
        source.officialPrices.map((official) => [
          field(official, '模型'),
          {
            businessId: `SRC-OFFICIAL-${slug(field(official, '模型'))}`,
            project: field(official, '模型'),
            valueOrRange: field(official, '分辨率'),
            unit: 'CNY/1M',
            asOf: '',
            sourceType: 'workbook',
            sourceName: 'sd收录.xlsx',
            reference: `${official.location.sheet}!${official.location.row}`,
            owner: '',
            note: field(official, '备注'),
            accessedAt: new Date().toISOString().slice(0, 10),
          },
        ])
      ).values(),
    ],
  ]
  const data: TemplateData = {
    channels,
    skus,
    sales,
    costs,
    mappings,
    profits,
    sources,
    issues,
  }
  data.issues = [...issues, ...validateTemplateData(data)]
  return data
}
