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
import { canonicalize } from 'json-canonicalize'

import { hashEntity, hashPayload } from './hash'
import { canonicalDecimal, normalizeEnum } from './normalize'
import type { CellSnapshot, ExtractedEntity, ExtractedWorkbook } from './types'

type ImportEntity = Record<string, unknown> & {
  business_id: string
  entity_hash: string
  source_ref: string
}

type ImportIssue = {
  code: string
  severity: 'error' | 'info' | 'warning'
  message: string
  entity_ref?: string
  row?: number
  sheet?: string
}

type ImportEntities = {
  channel_lines: ImportEntity[]
  channels: ImportEntity[]
  cost_rule_drafts: ImportEntity[]
  model_mappings: ImportEntity[]
  model_skus: ImportEntity[]
  route_blueprints: ImportEntity[]
  sale_proposals: ImportEntity[]
  sources: ImportEntity[]
  unresolved_variants: ImportEntity[]
}

export type ConfigImportDocument = {
  derived_preview: Record<string, never>
  entities: ImportEntities
  issues: ImportIssue[]
  kind: 'new-api.channel-config-import'
  manifest: {
    converter_version: string
    counts: Record<keyof ImportEntities, number>
    generated_at: string
    payload_sha256: string
    source_file_name: string
    source_sha256: string
    template_match: string
  }
  schema_version: 1
  template_version: '1' | '2'
}

export type ImportDocumentResult = {
  document: ConfigImportDocument
  hasFailures: boolean
  hasWarnings: boolean
}

export type BuildImportDocumentInput = {
  extracted: ExtractedWorkbook
  sourceBytes: Uint8Array
  sourceFileName: string
}

const V1_CHANNEL_TYPES: Record<string, number> = {
  'CH-4STOKEN': 1,
  'CH-8YES': 1,
  'CH-CANGYUANSUANLI': 66,
  'CH-CLMM': 63,
  'CH-DIMENSIO': 61,
  'CH-LUCEN': 64,
  'CH-MEGABYAI': 65,
  'CH-PAIPU': 67,
  'CH-SECURE': 68,
}

function cellText(cell: CellSnapshot | undefined): string {
  if (cell?.value === null || cell?.value === undefined) {
    return ''
  }
  if (cell.value instanceof Date) {
    return cell.value.toISOString()
  }
  return String(cell.value).trim()
}

function field(entity: ExtractedEntity, ...names: string[]): string {
  for (const name of names) {
    const value = cellText(entity.fields[name])
    if (value !== '') {
      return value
    }
  }
  return ''
}

function optionalDecimal(
  entity: ExtractedEntity,
  ...names: string[]
): string | undefined {
  const value = field(entity, ...names)
  return value === ''
    ? undefined
    : canonicalDecimal(value, { nonNegative: true })
}

function optionalBoolean(
  entity: ExtractedEntity,
  ...names: string[]
): boolean | undefined {
  for (const name of names) {
    const value = entity.fields[name]?.value
    if (typeof value === 'boolean') {
      return value
    }
    const normalized = cellText(entity.fields[name]).toLowerCase()
    if (['true', 'yes', '是'].includes(normalized)) {
      return true
    }
    if (['false', 'no', '否'].includes(normalized)) {
      return false
    }
  }
  return undefined
}

function optionalInteger(
  entity: ExtractedEntity,
  ...names: string[]
): number | undefined {
  const value = field(entity, ...names)
  if (value === '') {
    return undefined
  }
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) ? parsed : undefined
}

function listField(entity: ExtractedEntity, ...names: string[]): string[] {
  return field(entity, ...names)
    .split(/[|,]/)
    .map((value) => value.trim())
    .filter(Boolean)
}

function sourceRef(entity: ExtractedEntity): string {
  return field(entity, 'source_ref', '来源ID')
}

function sourceLocation(entity: ExtractedEntity): {
  raw_business_id: string
  row: number
  sheet: string
} {
  const location = entity.sourceLocations[0]
  return {
    raw_business_id: location?.businessId ?? entity.businessId,
    row: location?.row ?? 0,
    sheet: location?.sheet ?? '',
  }
}

async function authoritativeEntity(
  entity: ExtractedEntity,
  source: string,
  fields: Record<string, unknown>
): Promise<ImportEntity> {
  const location = sourceLocation(entity)
  const value: ImportEntity = {
    business_id: entity.businessId,
    source_ref: source,
    ...location,
    ...fields,
    entity_hash: '',
  }
  value.entity_hash = await hashEntity(value)
  return value
}

async function sourceHash(sourceBytes: Uint8Array): Promise<string> {
  const copy = new Uint8Array(new ArrayBuffer(sourceBytes.byteLength))
  copy.set(sourceBytes)
  const digest = await crypto.subtle.digest('SHA-256', copy)
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}

function sanitizedFileName(value: string): string {
  return value.split(/[\\/]/).at(-1)?.trim() || 'channel-config.xlsx'
}

function variantKey(entity: ExtractedEntity): string {
  const resolution = field(entity, 'resolution', '分辨率档位')
  return resolution === '' ? 'default' : normalizeEnum(resolution)
}

function pairingKey(entity: ExtractedEntity): string {
  return [
    entity.lineRef ?? field(entity, 'line_ref'),
    field(entity, 'upstream_model', '上游模型'),
    field(entity, 'sku_ref', 'SKU代码'),
  ].join('\u0000')
}

function channelType(channelRef: string): number | undefined {
  return V1_CHANNEL_TYPES[channelRef]
}

function lineDisplayName(entity: ExtractedEntity, channelName: string): string {
  return (
    field(entity, 'display_name') || `${channelName} / ${entity.businessId}`
  )
}

function providerHint(channelRef: string): string {
  return channelRef.replace(/^CH-/, '').toLowerCase()
}

function sourceIssue(
  code: string,
  severity: ImportIssue['severity'],
  message: string,
  entity?: ExtractedEntity
): ImportIssue {
  const location = entity ? sourceLocation(entity) : undefined
  return {
    code,
    severity,
    message,
    ...(entity ? { entity_ref: entity.businessId } : {}),
    ...(location?.sheet ? { sheet: location.sheet } : {}),
    ...(location?.row ? { row: location.row } : {}),
  }
}

export async function buildImportDocument(
  input: BuildImportDocumentInput
): Promise<ImportDocumentResult> {
  const issues: ImportIssue[] = []
  const entities: ImportEntities = {
    channels: [],
    channel_lines: [],
    model_skus: [],
    sale_proposals: [],
    cost_rule_drafts: [],
    model_mappings: [],
    route_blueprints: [],
    sources: [],
    unresolved_variants: [],
  }

  const sourceIDs = new Set(
    input.extracted.sources.map((source) => source.businessId)
  )
  for (const source of input.extracted.sources) {
    const location = sourceLocation(source)
    const url = field(source, 'url')
    const entity = await authoritativeEntity(source, source.businessId, {
      ...(url.startsWith('http://') || url.startsWith('https://')
        ? { url }
        : {}),
      audit_note: [
        field(source, 'name', '来源名称'),
        field(source, '引用位置'),
        field(source, 'note', '备注'),
      ]
        .filter(Boolean)
        .join('; '),
      raw_business_id: source.businessId,
      row: location.row,
      sheet: location.sheet,
    })
    entities.sources.push(entity)
  }

  const channelsByRef = new Map(
    input.extracted.channels.map((channel) => [channel.businessId, channel])
  )
  for (const channel of input.extracted.channels) {
    const source = sourceRef(channel)
    const type = channelType(channel.businessId)
    if (!sourceIDs.has(source) || type === undefined) {
      issues.push(
        sourceIssue(
          'CHANNEL_IDENTITY_UNRESOLVED',
          'error',
          'The channel has no supported channel identity.',
          channel
        )
      )
      continue
    }
    entities.channels.push(
      await authoritativeEntity(channel, source, {
        channel_type: type,
        display_name: field(channel, 'display_name', '渠道名称'),
        enabled: false,
        routing_enabled: false,
      })
    )
  }

  const lineIDs = new Set(
    input.extracted.channelLines.map((line) => line.businessId)
  )
  for (const line of input.extracted.channelLines) {
    const channel = channelsByRef.get(line.channelRef)
    const source = channel ? sourceRef(channel) : ''
    if (!channel || !sourceIDs.has(source)) {
      issues.push(
        sourceIssue(
          'LINE_CHANNEL_UNRESOLVED',
          'error',
          'The line has no source channel.',
          line
        )
      )
      continue
    }
    const realPerson =
      line.supportsRealPerson ?? optionalBoolean(line, 'supports_real_person')
    entities.channel_lines.push(
      await authoritativeEntity(line, source, {
        channel_ref: line.channelRef,
        display_name: lineDisplayName(
          line,
          field(channel, 'display_name', '渠道名称')
        ),
        line_ref: line.businessId,
        protocol: field(line, 'protocol') || 'task',
        provider_type_hint:
          field(line, 'provider_type_hint') || providerHint(line.channelRef),
        region: field(line, 'region') || 'global',
        ...(realPerson === undefined
          ? {}
          : { supports_real_person: realPerson }),
        status_proposal: 'disabled',
      })
    )
  }

  const skuIDs = new Set(input.extracted.modelSkus.map((sku) => sku.businessId))
  const skuByRef = new Map(
    input.extracted.modelSkus.map((sku) => [sku.businessId, sku])
  )
  for (const sku of input.extracted.modelSkus) {
    const source = sourceRef(sku)
    if (!sourceIDs.has(source)) {
      issues.push(
        sourceIssue(
          'SKU_SOURCE_UNRESOLVED',
          'error',
          'The SKU has no source record.',
          sku
        )
      )
      continue
    }
    const supportsRealPerson = optionalBoolean(
      sku,
      'supports_real_person',
      '支持真人脸'
    )
    entities.model_skus.push(
      await authoritativeEntity(sku, source, {
        ...(field(sku, '分辨率档位', 'resolution')
          ? { output_resolutions: [field(sku, '分辨率档位', 'resolution')] }
          : {}),
        ...(optionalDecimal(sku, 'duration_min', '最小时长秒')
          ? { duration_min: Number(field(sku, 'duration_min', '最小时长秒')) }
          : {}),
        ...(optionalDecimal(sku, 'duration_max', '最大时长秒')
          ? { duration_max: Number(field(sku, 'duration_max', '最大时长秒')) }
          : {}),
        ...(supportsRealPerson === undefined
          ? {}
          : { supports_real_person: supportsRealPerson }),
      })
    )
  }

  for (const sale of input.extracted.saleProposals) {
    const source = sourceRef(sale)
    const skuRef = field(sale, 'sku_ref', 'SKU代码')
    if (!sourceIDs.has(source) || !skuIDs.has(skuRef)) {
      issues.push(
        sourceIssue(
          'SALE_REFERENCE_UNRESOLVED',
          'error',
          'The sale proposal has an unresolved reference.',
          sale
        )
      )
      continue
    }
    const price = optionalDecimal(sale, 'native_unit_price', '原币/1M')
    entities.sale_proposals.push(
      await authoritativeEntity(sale, source, {
        billing_mode: field(sale, 'billing_mode', '计费模式'),
        currency: field(sale, 'currency', '币种'),
        enabled: false,
        model_sku_ref: skuRef,
        ...(price ? { total_per_million: price } : {}),
      })
    )
  }

  if (input.extracted.templateVersion === '2') {
    const mappingsByRouteTarget = new Map<string, ExtractedEntity[]>()
    for (const mapping of input.extracted.modelMappings) {
      const routeTargetRef = field(mapping, 'route_target_ref')
      const skuRef = field(mapping, 'sku_ref')
      const source = sourceRef(mapping)
      const lineRef = mapping.lineRef ?? field(mapping, 'line_ref')
      const sku = skuByRef.get(skuRef)
      if (
        !sku ||
        !lineIDs.has(lineRef) ||
        !sourceIDs.has(source) ||
        routeTargetRef === ''
      ) {
        issues.push(
          sourceIssue(
            'MAPPING_REFERENCE_UNRESOLVED',
            'error',
            'The mapping has an unresolved line, SKU, source, or route target.',
            mapping
          )
        )
        continue
      }
      const mappings = mappingsByRouteTarget.get(routeTargetRef) ?? []
      mappings.push(mapping)
      mappingsByRouteTarget.set(routeTargetRef, mappings)
      entities.model_mappings.push(
        await authoritativeEntity(mapping, source, {
          canonical_model: field(sku, 'canonical_model', '模型'),
          client_model: field(mapping, 'client_model', '客户端模型'),
          line_ref: lineRef,
          sku_ref: skuRef,
          upstream_model: field(mapping, 'upstream_model', '上游模型'),
        })
      )
    }

    for (const cost of input.extracted.costRuleDrafts) {
      const source = sourceRef(cost)
      const lineRef = cost.lineRef ?? field(cost, 'line_ref')
      const routeTargetRef = field(cost, 'route_target_ref')
      const variant = field(cost, 'cost_variant_key')
      if (
        !lineIDs.has(lineRef) ||
        !sourceIDs.has(source) ||
        routeTargetRef === '' ||
        variant === ''
      ) {
        issues.push(
          sourceIssue(
            'COST_REFERENCE_UNRESOLVED',
            'error',
            'The cost has an unresolved line, source, route target, or variant.',
            cost
          )
        )
        continue
      }
      const costMode = field(cost, 'cost_mode', '成本模式')
      const commonCostFields = {
        billing_multiplier: optionalDecimal(
          cost,
          'billing_multiplier',
          '计费倍率'
        ),
        currency: field(cost, 'currency', '币种'),
        currency_to_usd_rate: optionalDecimal(
          cost,
          'currency_to_usd_rate',
          '原币兑USD'
        ),
        fee_rate: optionalDecimal(cost, 'fee_rate', '手续费率'),
        purchase_discount_ratio: optionalDecimal(
          cost,
          'purchase_discount_ratio',
          '采购折扣'
        ),
        recharge_exchange_ratio: optionalDecimal(
          cost,
          'recharge_exchange_ratio',
          '充值兑换比例'
        ),
      }
      const costFields: Record<string, unknown> = {
        cost_mode: costMode,
        cost_variant_key: variant,
        line_ref: lineRef,
        route_target_ref: routeTargetRef,
        scenario: field(cost, 'scenario', '定价场景'),
        upstream_model: field(cost, 'upstream_model', '上游模型'),
        ...Object.fromEntries(
          Object.entries(commonCostFields).filter(
            ([, value]) => value !== undefined && value !== ''
          )
        ),
      }
      if (costMode === 'per_request') {
        costFields.unit_price = optionalDecimal(
          cost,
          'native_unit_price',
          '原币按次',
          '原币基础单价'
        )
      } else if (costMode === 'per_duration') {
        costFields.price_per_second = optionalDecimal(
          cost,
          'price_per_second',
          'native_unit_price',
          '原币/秒',
          '原币基础单价'
        )
      } else if (costMode === 'per_token') {
        costFields.total_per_million = optionalDecimal(
          cost,
          'total_per_million',
          'native_unit_price',
          '原币/1M',
          '原币基础单价'
        )
      }
      for (const [name, value] of Object.entries(costFields)) {
        if (value === undefined) delete costFields[name]
      }
      entities.cost_rule_drafts.push(
        await authoritativeEntity(cost, source, costFields)
      )
    }

    for (const route of input.extracted.routeBlueprints) {
      const routeTargetRef = route.businessId
      const mappings = mappingsByRouteTarget.get(routeTargetRef) ?? []
      const source = mappings.length > 0 ? sourceRef(mappings[0]) : ''
      const lineRef = route.lineRef ?? field(route, 'line_ref')
      const skuRef = field(route, 'sku_ref')
      const sku = skuByRef.get(skuRef)
      const variant = field(route, 'cost_variant_key')
      if (
        mappings.length === 0 ||
        !sourceIDs.has(source) ||
        !lineIDs.has(lineRef) ||
        !sku ||
        variant === ''
      ) {
        issues.push(
          sourceIssue(
            'ROUTE_TARGET_REFERENCE_UNRESOLVED',
            'error',
            'The route target has an unresolved mapping, line, SKU, source, or variant.',
            route
          )
        )
        continue
      }
      const line = input.extracted.channelLines.find(
        (candidate) => candidate.businessId === lineRef
      )
      const durationMin = optionalInteger(route, 'duration_min')
      const durationMax = optionalInteger(route, 'duration_max')
      const supportsRealPerson =
        line?.supportsRealPerson ??
        optionalBoolean(route, 'supports_real_person')
      const priority = optionalInteger(route, 'priority')
      entities.route_blueprints.push(
        await authoritativeEntity(
          {
            ...route,
            businessId: `route-blueprint/${routeTargetRef}`,
          },
          source,
          {
            canonical_model: field(route, 'canonical_model', '客户端模型'),
            client_model: field(route, 'client_model', '客户端模型'),
            merge_mode: field(route, 'merge_mode') || 'merge',
            model_mapping_refs: mappings.map((mapping) => mapping.businessId),
            targets: [
              {
                cost_variant_key: variant,
                enabled: false,
                line_ref: lineRef,
                ...(durationMin === undefined
                  ? {}
                  : { duration_min: durationMin }),
                ...(durationMax === undefined
                  ? {}
                  : { duration_max: durationMax }),
                ...(supportsRealPerson === undefined
                  ? {}
                  : { supports_real_person: supportsRealPerson }),
                output_resolutions: listField(route, 'output_resolutions'),
                route_target_ref: routeTargetRef,
                sku_ref: skuRef,
                upstream_model: field(route, 'upstream_model', '上游模型'),
                ...(priority === undefined ? {} : { priority }),
              },
            ],
          }
        )
      )
    }
  } else {
    const unresolvedKeys = new Set(
      input.extracted.unresolvedVariants.map(
        (variant) =>
          `${field(variant, 'channel_code')}/${field(variant, 'upstream_model')}`
      )
    )
    const costsByKey = new Map<string, ExtractedEntity[]>()
    const mappingsByKey = new Map<string, ExtractedEntity[]>()
    for (const cost of input.extracted.costRuleDrafts) {
      const key = `${field(cost, '渠道代码')}/${field(cost, 'upstream_model', '上游模型')}`
      if (cost.lineRef && !unresolvedKeys.has(key)) {
        const candidates = costsByKey.get(pairingKey(cost)) ?? []
        candidates.push(cost)
        costsByKey.set(pairingKey(cost), candidates)
      }
    }
    for (const mapping of input.extracted.modelMappings) {
      const key = `${field(mapping, '渠道代码')}/${field(mapping, 'upstream_model', '上游模型')}`
      if (mapping.lineRef && !unresolvedKeys.has(key)) {
        const candidates = mappingsByKey.get(pairingKey(mapping)) ?? []
        candidates.push(mapping)
        mappingsByKey.set(pairingKey(mapping), candidates)
      }
    }

    for (const [key, costs] of costsByKey) {
      const mappings = mappingsByKey.get(key) ?? []
      costs.sort((left, right) =>
        left.businessId.localeCompare(right.businessId)
      )
      mappings.sort((left, right) =>
        left.businessId.localeCompare(right.businessId)
      )
      if (costs.length !== mappings.length) {
        issues.push(
          sourceIssue(
            'COST_MAPPING_MISMATCH',
            'error',
            'Costs and mappings cannot be paired.',
            costs[0]
          )
        )
        continue
      }
      for (const [index, cost] of costs.entries()) {
        const mapping = mappings[index]
        const skuRef = field(cost, 'sku_ref', 'SKU代码')
        const sku = skuByRef.get(skuRef)
        const lineRef = cost.lineRef ?? ''
        const source = sourceRef(cost)
        if (!sku || !lineIDs.has(lineRef) || !sourceIDs.has(source)) {
          issues.push(
            sourceIssue(
              'COST_REFERENCE_UNRESOLVED',
              'error',
              'The cost has an unresolved reference.',
              cost
            )
          )
          continue
        }
        const targetRef = `route-target/${mapping.businessId}`
        const variant = variantKey(sku)
        const costMode = field(cost, 'cost_mode', '成本模式')
        const commonCostFields = {
          billing_multiplier: optionalDecimal(
            cost,
            'billing_multiplier',
            '计费倍率'
          ),
          currency: field(cost, 'currency', '币种'),
          currency_to_usd_rate: optionalDecimal(
            cost,
            'currency_to_usd_rate',
            '原币兑USD'
          ),
          fee_rate: optionalDecimal(cost, 'fee_rate', '手续费率'),
          purchase_discount_ratio: optionalDecimal(
            cost,
            'purchase_discount_ratio',
            '采购折扣'
          ),
          recharge_exchange_ratio: optionalDecimal(
            cost,
            'recharge_exchange_ratio',
            '充值兑换比例'
          ),
        }
        const costFields: Record<string, unknown> = {
          cost_mode: costMode,
          cost_variant_key: variant,
          line_ref: lineRef,
          route_target_ref: targetRef,
          scenario: field(cost, 'scenario', '定价场景'),
          upstream_model: field(cost, 'upstream_model', '上游模型'),
          ...Object.fromEntries(
            Object.entries(commonCostFields).filter(
              ([, value]) => value !== undefined && value !== ''
            )
          ),
        }
        if (costMode === 'per_request') {
          costFields.unit_price = optionalDecimal(
            cost,
            'native_unit_price',
            '原币按次',
            '原币基础单价'
          )
        } else if (costMode === 'per_duration') {
          costFields.price_per_second = optionalDecimal(
            cost,
            'price_per_second',
            '原币/秒',
            '原币基础单价'
          )
        } else if (costMode === 'per_token') {
          costFields.total_per_million = optionalDecimal(
            cost,
            'total_per_million',
            '原币/1M',
            '原币基础单价'
          )
        }
        for (const [name, value] of Object.entries(costFields)) {
          if (value === undefined) delete costFields[name]
        }
        entities.cost_rule_drafts.push(
          await authoritativeEntity(cost, source, costFields)
        )

        const mappingSource = sourceRef(mapping)
        if (!sourceIDs.has(mappingSource)) {
          issues.push(
            sourceIssue(
              'MAPPING_SOURCE_UNRESOLVED',
              'error',
              'The mapping has no source record.',
              mapping
            )
          )
          continue
        }
        const canonicalModel = field(sku, 'canonical_model', '模型')
        entities.model_mappings.push(
          await authoritativeEntity(mapping, mappingSource, {
            canonical_model: canonicalModel,
            client_model: field(mapping, 'client_model', '客户端模型'),
            line_ref: lineRef,
            sku_ref: skuRef,
            upstream_model: field(mapping, 'upstream_model', '上游模型'),
          })
        )
        const durationMin = field(sku, 'duration_min', '最小时长秒')
        const durationMax = field(sku, 'duration_max', '最大时长秒')
        const line = input.extracted.channelLines.find(
          (candidate) => candidate.businessId === lineRef
        )
        entities.route_blueprints.push(
          await authoritativeEntity(
            {
              ...mapping,
              businessId: `route-blueprint/${mapping.businessId}`,
            },
            mappingSource,
            {
              canonical_model: canonicalModel,
              client_model: field(mapping, 'client_model', '客户端模型'),
              merge_mode: 'merge',
              model_mapping_refs: [mapping.businessId],
              targets: [
                {
                  cost_variant_key: variant,
                  enabled: false,
                  line_ref: lineRef,
                  ...(durationMin ? { duration_min: Number(durationMin) } : {}),
                  ...(durationMax ? { duration_max: Number(durationMax) } : {}),
                  ...(line?.supportsRealPerson === undefined
                    ? {}
                    : { supports_real_person: line.supportsRealPerson }),
                  output_resolutions: [field(sku, 'resolution', '分辨率档位')],
                  route_target_ref: targetRef,
                  sku_ref: skuRef,
                  upstream_model: field(mapping, 'upstream_model', '上游模型'),
                },
              ],
            }
          )
        )
      }
    }
  }

  for (const unresolved of input.extracted.unresolvedVariants) {
    const source = input.extracted.channels.find(
      (channel) => channel.businessId === field(unresolved, 'channel_code')
    )
    const sourceRefValue = source ? sourceRef(source) : ''
    if (!sourceIDs.has(sourceRefValue)) {
      issues.push(
        sourceIssue(
          'UNRESOLVED_VARIANT_SOURCE',
          'error',
          'The unresolved variant has no source record.',
          unresolved
        )
      )
      continue
    }
    entities.unresolved_variants.push(
      await authoritativeEntity(unresolved, sourceRefValue, {
        line_ref: '',
        reason:
          'The workbook does not provide a verified line identity for this price conflict.',
        upstream_model: field(unresolved, 'upstream_model'),
      })
    )
    issues.push(
      sourceIssue(
        'COST_VARIANT_AMBIGUOUS',
        'warning',
        'This variant needs a verified channel line before it can be published.',
        unresolved
      )
    )
  }

  for (const collection of Object.values(entities)) {
    collection.sort((left, right) =>
      left.business_id.localeCompare(right.business_id)
    )
  }
  const document: ConfigImportDocument = {
    kind: 'new-api.channel-config-import',
    schema_version: 1,
    template_version: input.extracted.templateVersion,
    manifest: {
      source_file_name: sanitizedFileName(input.sourceFileName),
      source_sha256: await sourceHash(input.sourceBytes),
      payload_sha256: '',
      generated_at: new Date().toISOString(),
      converter_version: '1.0.0',
      template_match: `v${input.extracted.templateVersion}`,
      counts: Object.fromEntries(
        Object.entries(entities).map(([name, collection]) => [
          name,
          collection.length,
        ])
      ) as ConfigImportDocument['manifest']['counts'],
    },
    entities,
    derived_preview: {},
    issues,
  }
  document.manifest.payload_sha256 = await hashPayload(document)
  return {
    document,
    hasFailures: issues.some((issue) => issue.severity === 'error'),
    hasWarnings: issues.some((issue) => issue.severity === 'warning'),
  }
}

export function serializeImportDocument(
  document: ConfigImportDocument
): string {
  return `${canonicalize(document)}\n`
}
