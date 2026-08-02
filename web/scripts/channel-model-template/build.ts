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
  RowOverride,
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

type ReferenceContract = {
  images: number
  videos: number
  audios: number
  totalMax: number
  videoAudioTotalMax: number | null
  videoDurationSeconds: number
  minimumImages: number
  modes: string[]
  aspectRatios: string[]
}

function field(record: SourceRecord | undefined, name: string): string {
  if (!record) return ''
  return cellText(record.fields[name] ?? null)
}

function numericField(
  record: SourceRecord | undefined,
  name: string
): Decimal | null {
  if (!record) return null
  const text = field(record, name).replaceAll(/[%￥¥,]/gu, '')
  if (text === '') return null
  try {
    const value = new Decimal(text)
    return value.isFinite() ? value : null
  } catch {
    return null
  }
}

function referenceContract(record: SourceRecord): {
  contract: ReferenceContract | null
  error: string
  structured: boolean
} {
  const structured = Object.hasOwn(record.fields, '参考图数')
  if (!structured) {
    const legacy = field(record, '素材限制')
    if (!/^\d{3}$/u.test(legacy)) {
      return { contract: null, error: '', structured: false }
    }
    const [images, videos, audios] = [...legacy].map(Number)
    return {
      contract: {
        images,
        videos,
        audios,
        totalMax: images + videos + audios,
        videoAudioTotalMax: null,
        videoDurationSeconds: videos > 0 ? 15 : 0,
        minimumImages: 0,
        modes: [],
        aspectRatios: [],
      },
      error: '',
      structured: false,
    }
  }

  const integer = (name: string, optional = false): number | null => {
    const value = field(record, name)
    if (value === '' && optional) return null
    if (value === '') return null
    const parsed = Number(value)
    return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : null
  }
  const images = integer('参考图数')
  const videos = integer('参考视频数')
  const audios = integer('参考音频数')
  const totalMax = integer('最大素材数')
  const videoAudioTotalMax = integer('视频音频合计上限', true)
  const videoDurationSeconds = integer('参考视频总时长上限 秒', true)
  const minimumImages = integer('最小参考图数')
  if (
    images === null ||
    videos === null ||
    audios === null ||
    totalMax === null ||
    minimumImages === null ||
    (videos > 0 && videoDurationSeconds === null)
  ) {
    return { contract: null, error: '', structured: true }
  }
  if (minimumImages > images) {
    return {
      contract: null,
      error: '最小参考图数不能超过参考图数',
      structured: true,
    }
  }
  if (field(record, '视频输入') === '否' && videos > 0) {
    return {
      contract: null,
      error: '视频输入为否时参考视频数必须为0',
      structured: true,
    }
  }
  if (totalMax > images + videos + audios) {
    return {
      contract: null,
      error: '最大素材数超过独立素材上限之和',
      structured: true,
    }
  }
  if (videoAudioTotalMax !== null) {
    if (videoAudioTotalMax > videos + audios) {
      return {
        contract: null,
        error: '视频音频合计上限超过独立上限之和',
        structured: true,
      }
    }
  }
  const sourceChannel = field(record, '渠道')
  const group = field(record, '上游模型分组')
  let modes: string[] = []
  if (sourceChannel === '7') {
    modes = ['first_last_frames', 'omni_reference', 'agentic']
  } else if (sourceChannel === '5' && group === 'video-海外') {
    modes = ['first_last_frames', 'omni_reference']
  }
  const rawAspectRatio = field(record, '比例').toLowerCase()
  const aspectRatios = (
    rawAspectRatio === 'auto' && modes.length > 0
      ? ['1:1', '4:3', '3:4', '16:9', '9:16', '21:9']
      : rawAspectRatio.split(',')
  )
    .map((value) => value.trim())
    .filter((value) => value !== '' && value !== 'auto')
  const effectiveTotalMax = Math.min(
    totalMax,
    images + (videoAudioTotalMax ?? videos + audios)
  )
  return {
    contract: {
      images,
      videos,
      audios,
      totalMax: effectiveTotalMax,
      videoAudioTotalMax,
      videoDurationSeconds: videos === 0 ? 0 : (videoDurationSeconds ?? 0),
      minimumImages,
      modes,
      aspectRatios,
    },
    error: '',
    structured: true,
  }
}

function slug(value: string): string {
  return value
    .trim()
    .toUpperCase()
    .replaceAll(/[^A-Z0-9]+/gu, '-')
    .replaceAll(/^-+|-+$/gu, '')
}

function resolution(value: string): string {
  const normalized = value.trim().toLowerCase()
  if (normalized === '') return 'default'
  if (normalized === '2160p') return '4k'
  return normalized.endsWith('p') || normalized === '4k'
    ? normalized
    : `${normalized}p`
}

function parseDuration(value: string): [number, number] {
  const excelDate = value.match(/^\d{4}-(\d{1,2})-(\d{1,2})(?:T|$)/u)
  if (excelDate) return [Number(excelDate[1]), Number(excelDate[2])]
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

function inferClientModel(rawModel: string, officialModels: string[]): string {
  const available = new Set(officialModels)
  const normalized = rawModel.toLowerCase()
  let suffix = ''
  if (normalized.includes('mini')) {
    suffix = '-mini'
  } else if (normalized.includes('fast')) {
    suffix = '-fast'
  }
  const candidate = `seedance-2.0${suffix}`
  if (available.has(candidate)) return candidate
  if (available.has('seedance-2.0')) return 'seedance-2.0'
  return rawModel
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

function fallbackSkuDimensions(resolutionValue: string): [number, number] {
  if (resolutionValue === '480p') return [864, 496]
  if (resolutionValue === '720p') return [1280, 720]
  if (resolutionValue === '1080p') return [1920, 1080]
  if (resolutionValue === '4k') return [3840, 2160]
  return [1, 1]
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
    const modelRule = modelRuleFor(
      rawModel,
      rules,
      field(official, '模型') ||
        inferClientModel(
          rawModel,
          source.officialPrices.map((price) => field(price, '模型'))
        )
    )
    const model = modelRule.clientModel ?? field(official, '模型') ?? rawModel
    const resolutionValue = resolution(field(modelRecord, '清晰度'))
    const officialPrice = findOfficial(officialIndex, model, resolutionValue)
    const version =
      field(officialPrice, '版本') || field(modelRecord, '版本') || '标准'
    const [minDuration, maxDuration] = parseDuration(
      field(modelRecord, '时长范围')
    )
    const [ruleMin, ruleMax] = [
      modelRule.minDurationSeconds ?? minDuration,
      modelRule.maxDurationSeconds ?? maxDuration,
    ]
    const id = skuId(model, version, resolutionValue)
    if (rows.has(id)) continue
    const [fallbackWidth, fallbackHeight] =
      fallbackSkuDimensions(resolutionValue)
    const width =
      modelRule.outputWidth ??
      (Number(field(officialPrice, '长边')) || fallbackWidth)
    const height =
      modelRule.outputHeight ??
      (Number(field(officialPrice, '短边')) || fallbackHeight)
    const frameRate =
      modelRule.frameRate ?? (Number(field(officialPrice, '帧率')) || 24)
    const channelSource = source.channels.find(
      (channel) => field(channel, '渠道') === field(modelRecord, '渠道')
    )
    let sourceId = 'SRC-SD-IMPORT'
    if (officialPrice) {
      sourceId = `SRC-OFFICIAL-${slug(model)}`
    } else if (channelSource) {
      sourceId = sourceIdForChannel(channelSource)
    }
    if (!officialPrice) {
      issues.push(
        issue(
          'SKU_UNRESOLVED',
          'WARN',
          `No official SKU matched ${model}/${resolutionValue}.`,
          modelRecord,
          id
        )
      )
    }
    let supportsRealPerson = '待确认'
    if (modelRule.supportsRealPerson !== undefined) {
      supportsRealPerson = modelRule.supportsRealPerson ? '是' : '否'
    }
    let supportsSuperResolution = '待确认'
    if (modelRule.supportsSuperResolution !== undefined) {
      supportsSuperResolution = modelRule.supportsSuperResolution ? '是' : '否'
    }
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
      supportsRealPerson,
      supportsSuperResolution,
      measurementMethod: 'video_pixel_tokens',
      status: officialPrice ? 'active' : 'draft',
      sourceId,
      sourceSheet: officialPrice?.location.sheet ?? modelRecord.location.sheet,
      sourceRow: officialPrice?.location.row ?? modelRecord.location.row,
      note: officialPrice
        ? '由官方价格矩阵和源模型能力合并生成。'
        : '缺少官方价格，已生成 draft SKU。',
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

function overridePrice(
  override: RowOverride | undefined,
  mode: CostMode
): Decimal | null {
  let value = override?.nativePerMillion
  if (mode === 'per_duration') {
    value = override?.nativePerSecond
  } else if (mode === 'per_request') {
    value = override?.nativePerRequest
  }
  if (!value) return null
  try {
    const decimal = new Decimal(value)
    return decimal.isFinite() ? decimal : null
  } catch {
    return null
  }
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
    const modelRule = modelRuleFor(
      rawModel,
      rules,
      inferClientModel(
        rawModel,
        source.officialPrices.map((price) => field(price, '模型'))
      )
    )
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
    const reference = referenceContract(record)
    const hasReferenceContract = reference.contract !== null
    const referenceBusinessValid = hasReferenceContract
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
    const native =
      overridePrice(override, effectiveMode) ??
      priceForMode(record, effectiveMode)
    const hasValidNativePrice = native !== null && native.gt(0)
    if (!hasValidNativePrice) {
      issues.push(
        issue(
          'COST_PRICE_INVALID',
          'WARN',
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
    if (!hasReferenceContract) {
      let code = 'MATERIAL_LIMIT_UNRESOLVED'
      if (reference.error) {
        code = 'REFERENCE_CONTRACT_INVALID'
      } else if (reference.structured) {
        code = 'REFERENCE_CONTRACT_UNRESOLVED'
      }
      const message =
        reference.error ||
        (reference.structured
          ? '结构化素材合同字段必须完整且为非负整数。'
          : 'A verified three-digit 素材限制 value is required for the V1 mapping.')
      issues.push(
        issue(code, reference.error ? 'FAIL' : 'WARN', message, record)
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
    const sourceStatus: 'active' | 'draft' =
      field(record, '状态') === '正常' ? 'active' : 'draft'
    const costStatus =
      sku?.status === 'active' && referenceBusinessValid && hasValidNativePrice
        ? (override?.status ?? sourceStatus)
        : 'draft'
    let supportsRealPerson = field(record, '过真人脸')
    if (override?.supportsRealPerson !== undefined) {
      supportsRealPerson = override.supportsRealPerson ? '是' : '否'
    }
    for (const scenario of ['no_video', 'with_video'] as const) {
      const suffix = scenarioCode(scenario)
      let billingCode = 'TOK'
      if (effectiveMode === 'per_duration') {
        billingCode = 'DUR'
      } else if (effectiveMode === 'per_request') {
        billingCode = 'REQ'
      }
      let unit = 'USD/1M tokens'
      if (effectiveMode === 'per_duration') {
        unit = 'USD/second'
      } else if (effectiveMode === 'per_request') {
        unit = 'USD/request'
      }
      costs.push({
        businessId: `COST-${slug(channelCode.replace(/^CH-/, ''))}-${baseId}-${resolutionId}-${billingCode}-${suffix}`,
        channelCode,
        upstreamModel,
        skuCode,
        scenario,
        mode: effectiveMode,
        tokenSubMode: effectiveMode === 'per_token' ? 'total_tokens' : '',
        meterSource: effectiveMode === 'per_token' ? 'upstream_usage' : '',
        tokenField: effectiveMode === 'per_token' ? 'total_tokens' : '',
        chargeEvent: 'task_succeeded',
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
        unit,
        status: costStatus,
        sourceId,
        sourceSheet: record.location.sheet,
        sourceRow: record.location.row,
        note: `时长=${minDuration}-${maxDuration}; 原模型=${rawModel}; 上游模型分组=${field(record, '上游模型分组')}; 真人脸=${supportsRealPerson}; 原备注=${field(record, '备注')}`,
      })
    }
    const referenceNote = reference.structured
      ? [
          `参考图数=${reference.contract?.images ?? ''}`,
          `参考视频数=${reference.contract?.videos ?? ''}`,
          `参考音频数=${reference.contract?.audios ?? ''}`,
          `最大素材数=${reference.contract?.totalMax ?? ''}`,
          `视频音频合计上限=${reference.contract?.videoAudioTotalMax ?? ''}`,
          `素材模式=${reference.contract?.modes.join(',') ?? ''}`,
          `归一化比例=${reference.contract?.aspectRatios.join(',') ?? ''}`,
          `参考视频总时长上限秒=${reference.contract?.videoDurationSeconds ?? ''}`,
          `最小参考图数=${reference.contract?.minimumImages ?? ''}`,
        ]
      : [`素材限制=${field(record, '素材限制')}`]
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
      note: [
        `原模型=${rawModel}`,
        ...referenceNote,
        `上游模型分组=${field(record, '上游模型分组')}`,
        `原比例=${field(record, '比例')}`,
        `真人脸=${supportsRealPerson}`,
      ].join('; '),
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
