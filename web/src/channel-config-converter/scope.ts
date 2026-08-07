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
import type {
  ConfigImportDocument,
  ImportEntities,
  ImportEntity,
  ImportIssue,
} from './document'
import { hashEntity, hashPayload } from './hash'

const entityCollectionNames = [
  'channels',
  'channel_lines',
  'cost_rule_drafts',
  'group_routing_requirements',
  'model_mappings',
  'model_skus',
  'route_blueprints',
  'sale_proposals',
  'sources',
  'unresolved_variants',
] as const

export type ChannelLineGroup = {
  channel: ImportEntity
  lines: ImportEntity[]
}

export type ScopeValidationError =
  | 'DANGLING_REFERENCE'
  | 'EMPTY_ROUTE_BLUEPRINT'
  | 'EMPTY_SELECTION'
  | 'UNKNOWN_LINE_REF'

export type ScopedImportDocumentResult = {
  blockingIssues: ImportIssue[]
  canUse: boolean
  document: ConfigImportDocument
  groups: ChannelLineGroup[]
  selectedGroupCount: number
  selectedLineCount: number
  validationErrors: ScopeValidationError[]
  warnings: ImportIssue[]
}

function stringField(entity: ImportEntity, field: string): string {
  const value = entity[field]
  return typeof value === 'string' ? value : ''
}

function stringList(entity: ImportEntity, field: string): string[] {
  const value = entity[field]
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function entityMap(entities: ImportEntity[]): Map<string, ImportEntity> {
  return new Map(entities.map((entity) => [entity.business_id, entity]))
}

function emptyEntities(): ImportEntities {
  return {
    channels: [],
    channel_lines: [],
    cost_rule_drafts: [],
    group_routing_requirements: [],
    model_mappings: [],
    model_skus: [],
    route_blueprints: [],
    sale_proposals: [],
    sources: [],
    unresolved_variants: [],
  }
}

function cloneEntity(entity: ImportEntity): ImportEntity {
  return structuredClone(entity)
}

function sortEntities(entities: ImportEntities): void {
  for (const name of entityCollectionNames) {
    entities[name].sort((left, right) =>
      left.business_id.localeCompare(right.business_id)
    )
  }
}

function addValidationError(
  validationErrors: ScopeValidationError[],
  error: ScopeValidationError
): void {
  if (!validationErrors.includes(error)) validationErrors.push(error)
}

function documentWithScope(
  original: ConfigImportDocument,
  entities: ImportEntities,
  issues: ImportIssue[]
): ConfigImportDocument {
  const document = structuredClone(original)
  document.entities = entities
  document.issues = issues
  document.manifest.counts = Object.fromEntries(
    entityCollectionNames.map((name) => [name, entities[name].length])
  ) as ConfigImportDocument['manifest']['counts']
  return document
}

function validateScopeReferences(
  entities: ImportEntities,
  validationErrors: ScopeValidationError[]
): void {
  const sources = entityMap(entities.sources)
  const channels = entityMap(entities.channels)
  const lines = entityMap(entities.channel_lines)
  const skus = entityMap(entities.model_skus)
  const mappings = entityMap(entities.model_mappings)
  const routeTargets = new Map<string, ImportEntity>()

  for (const route of entities.route_blueprints) {
    const targets = route.targets
    if (!Array.isArray(targets) || targets.length === 0) {
      addValidationError(validationErrors, 'EMPTY_ROUTE_BLUEPRINT')
      continue
    }
    for (const target of targets) {
      if (
        typeof target !== 'object' ||
        target === null ||
        Array.isArray(target)
      ) {
        addValidationError(validationErrors, 'DANGLING_REFERENCE')
        continue
      }
      const targetEntity = target as ImportEntity
      const routeTargetRef = stringField(targetEntity, 'route_target_ref')
      if (routeTargetRef === '' || routeTargets.has(routeTargetRef)) {
        addValidationError(validationErrors, 'DANGLING_REFERENCE')
        continue
      }
      routeTargets.set(routeTargetRef, targetEntity)
    }
  }

  const hasSource = (entity: ImportEntity) =>
    sources.has(stringField(entity, 'source_ref'))
  const hasLine = (lineRef: string) => lines.has(lineRef)
  const hasSKU = (skuRef: string) => skus.has(skuRef)

  for (const collection of entityCollectionNames) {
    for (const entity of entities[collection]) {
      if (!hasSource(entity)) {
        addValidationError(validationErrors, 'DANGLING_REFERENCE')
      }
    }
  }
  for (const line of entities.channel_lines) {
    if (!channels.has(stringField(line, 'channel_ref'))) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }
  }
  for (const proposal of entities.sale_proposals) {
    if (!hasSKU(stringField(proposal, 'model_sku_ref'))) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }
  }
  for (const mapping of entities.model_mappings) {
    if (
      !hasLine(stringField(mapping, 'line_ref')) ||
      !hasSKU(stringField(mapping, 'sku_ref'))
    ) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }
  }
  for (const cost of entities.cost_rule_drafts) {
    const target = routeTargets.get(stringField(cost, 'route_target_ref'))
    if (!hasLine(stringField(cost, 'line_ref')) || !target) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
      continue
    }
    if (
      stringField(target, 'line_ref') !== stringField(cost, 'line_ref') ||
      stringField(target, 'cost_variant_key') !==
        stringField(cost, 'cost_variant_key') ||
      stringField(target, 'upstream_model') !==
        stringField(cost, 'upstream_model')
    ) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }
  }
  for (const route of entities.route_blueprints) {
    for (const mappingRef of stringList(route, 'model_mapping_refs')) {
      if (!mappings.has(mappingRef)) {
        addValidationError(validationErrors, 'DANGLING_REFERENCE')
      }
    }
    for (const target of route.targets as ImportEntity[]) {
      if (
        !hasLine(stringField(target, 'line_ref')) ||
        !hasSKU(stringField(target, 'sku_ref'))
      ) {
        addValidationError(validationErrors, 'DANGLING_REFERENCE')
      }
    }
  }
  for (const variant of entities.unresolved_variants) {
    const lineRef = stringField(variant, 'line_ref')
    if (lineRef !== '' && !hasLine(lineRef)) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }
  }
}

export function groupChannelLines(
  document: ConfigImportDocument
): ChannelLineGroup[] {
  const channels = entityMap(document.entities.channels)
  const linesByChannel = new Map<string, ImportEntity[]>()
  for (const line of document.entities.channel_lines) {
    const channelRef = stringField(line, 'channel_ref')
    const lines = linesByChannel.get(channelRef) ?? []
    lines.push(line)
    linesByChannel.set(channelRef, lines)
  }
  return [...linesByChannel.entries()]
    .flatMap(([channelRef, lines]) => {
      const channel = channels.get(channelRef)
      return channel ? [{ channel, lines: [...lines] }] : []
    })
    .sort((left, right) =>
      left.channel.business_id.localeCompare(right.channel.business_id)
    )
    .map((group) => ({
      ...group,
      lines: group.lines.sort((left, right) =>
        left.business_id.localeCompare(right.business_id)
      ),
    }))
}

export async function buildScopedImportDocument(
  original: ConfigImportDocument,
  selectedLineRefs: ReadonlySet<string>
): Promise<ScopedImportDocumentResult> {
  const validationErrors: ScopeValidationError[] = []
  const groups = groupChannelLines(original)
  const linesByRef = entityMap(original.entities.channel_lines)
  const selectedLineIDs = new Set(selectedLineRefs)
  if (selectedLineIDs.size === 0) {
    addValidationError(validationErrors, 'EMPTY_SELECTION')
  }
  if ([...selectedLineIDs].some((lineRef) => !linesByRef.has(lineRef))) {
    addValidationError(validationErrors, 'UNKNOWN_LINE_REF')
  }

  const entities = emptyEntities()
  if (validationErrors.length === 0) {
    const selectedChannelRefs = new Set<string>()
    entities.channel_lines = original.entities.channel_lines
      .filter((line) => selectedLineIDs.has(stringField(line, 'line_ref')))
      .map(cloneEntity)
    for (const line of entities.channel_lines) {
      selectedChannelRefs.add(stringField(line, 'channel_ref'))
    }
    entities.channels = original.entities.channels
      .filter((channel) => selectedChannelRefs.has(channel.business_id))
      .map(cloneEntity)
    if (entities.channels.length !== selectedChannelRefs.size) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }

    entities.cost_rule_drafts = original.entities.cost_rule_drafts
      .filter((cost) => selectedLineIDs.has(stringField(cost, 'line_ref')))
      .map(cloneEntity)
    entities.model_mappings = original.entities.model_mappings
      .filter((mapping) =>
        selectedLineIDs.has(stringField(mapping, 'line_ref'))
      )
      .map(cloneEntity)
    const selectedMappingIDs = new Set(
      entities.model_mappings.map((mapping) => mapping.business_id)
    )
    const routeTargetRefs = new Set<string>()

    for (const originalRoute of original.entities.route_blueprints) {
      const targets = Array.isArray(originalRoute.targets)
        ? (originalRoute.targets as ImportEntity[])
        : []
      const selectedTargets = targets
        .filter((target) =>
          selectedLineIDs.has(stringField(target, 'line_ref'))
        )
        .map(cloneEntity)
        .sort((left, right) => {
          const targetOrder = stringField(
            left,
            'route_target_ref'
          ).localeCompare(stringField(right, 'route_target_ref'))
          if (targetOrder !== 0) return targetOrder
          const lineOrder = stringField(left, 'line_ref').localeCompare(
            stringField(right, 'line_ref')
          )
          if (lineOrder !== 0) return lineOrder
          return stringField(left, 'sku_ref').localeCompare(
            stringField(right, 'sku_ref')
          )
        })
      if (selectedTargets.length === 0) continue

      const route = cloneEntity(originalRoute)
      route.targets = selectedTargets
      route.model_mapping_refs = stringList(route, 'model_mapping_refs')
        .filter((mappingRef) => selectedMappingIDs.has(mappingRef))
        .sort((left, right) => left.localeCompare(right))
      route.entity_hash = await hashEntity(route)
      entities.route_blueprints.push(route)
      for (const target of selectedTargets) {
        routeTargetRefs.add(stringField(target, 'route_target_ref'))
      }
    }

    const selectedGroupNames = new Set(
      entities.route_blueprints.map(
        (route) => stringField(route, 'group_name') || 'default'
      )
    )
    entities.group_routing_requirements = (
      original.entities.group_routing_requirements ?? []
    )
      .filter((requirement) =>
        selectedGroupNames.has(stringField(requirement, 'group_name'))
      )
      .map(cloneEntity)

    const selectedSKURefs = new Set<string>()
    for (const mapping of entities.model_mappings) {
      selectedSKURefs.add(stringField(mapping, 'sku_ref'))
    }
    for (const route of entities.route_blueprints) {
      for (const target of route.targets as ImportEntity[]) {
        selectedSKURefs.add(stringField(target, 'sku_ref'))
      }
    }
    entities.model_skus = original.entities.model_skus
      .filter((sku) => selectedSKURefs.has(sku.business_id))
      .map(cloneEntity)
    entities.sale_proposals = original.entities.sale_proposals
      .filter((proposal) =>
        selectedSKURefs.has(stringField(proposal, 'model_sku_ref'))
      )
      .map(cloneEntity)
    entities.unresolved_variants = original.entities.unresolved_variants
      .filter((variant) => {
        const lineRef = stringField(variant, 'line_ref')
        return lineRef !== '' && selectedLineIDs.has(lineRef)
      })
      .map(cloneEntity)

    const lineBoundIssueRefs = new Set<string>(selectedLineIDs)
    for (const entity of [
      ...entities.cost_rule_drafts,
      ...entities.model_mappings,
      ...entities.group_routing_requirements,
      ...entities.unresolved_variants,
    ]) {
      lineBoundIssueRefs.add(entity.business_id)
    }
    const issues = original.issues
      .filter(
        (issue) =>
          issue.entity_ref !== undefined &&
          lineBoundIssueRefs.has(issue.entity_ref)
      )
      .map((issue) => structuredClone(issue))

    const sourcesByRef = entityMap(original.entities.sources)
    const sourceRefs = new Set<string>()
    for (const collection of entityCollectionNames) {
      if (collection === 'sources') continue
      for (const entity of entities[collection]) {
        sourceRefs.add(stringField(entity, 'source_ref'))
      }
    }
    for (const sourceRef of sourceRefs) {
      let currentRef = sourceRef
      const visited = new Set<string>()
      while (currentRef !== '' && !visited.has(currentRef)) {
        visited.add(currentRef)
        const source = sourcesByRef.get(currentRef)
        if (!source) {
          addValidationError(validationErrors, 'DANGLING_REFERENCE')
          break
        }
        entities.sources.push(cloneEntity(source))
        currentRef = stringField(source, 'source_ref')
      }
    }
    entities.sources = [...entityMap(entities.sources).values()]

    const costTargets = new Set(
      entities.cost_rule_drafts.map((cost) =>
        stringField(cost, 'route_target_ref')
      )
    )
    if ([...costTargets].some((targetRef) => !routeTargetRefs.has(targetRef))) {
      addValidationError(validationErrors, 'DANGLING_REFERENCE')
    }

    sortEntities(entities)
    const document = documentWithScope(original, entities, issues)
    validateScopeReferences(entities, validationErrors)
    document.manifest.payload_sha256 = await hashPayload(document)
    const blockingIssues = document.issues.filter(
      (issue) => issue.severity === 'error'
    )
    const warnings = document.issues.filter(
      (issue) => issue.severity === 'warning'
    )
    return {
      blockingIssues,
      canUse: validationErrors.length === 0 && blockingIssues.length === 0,
      document,
      groups,
      selectedGroupCount: groups.filter((group) =>
        group.lines.some((line) =>
          selectedLineIDs.has(stringField(line, 'line_ref'))
        )
      ).length,
      selectedLineCount: selectedLineIDs.size,
      validationErrors,
      warnings,
    }
  }

  const document = documentWithScope(original, entities, [])
  document.manifest.payload_sha256 = await hashPayload(document)
  return {
    blockingIssues: [],
    canUse: false,
    document,
    groups,
    selectedGroupCount: 0,
    selectedLineCount: 0,
    validationErrors,
    warnings: [],
  }
}
