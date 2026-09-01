export type ImagePreviewEndpoint = {
  capability?: {
    enabled?: boolean
    sizes?: string[]
    qualities?: string[]
    response_formats?: string[]
    max_n?: number
  }
}

export type ImagePreviewModel = {
  endpoints?: Record<string, ImagePreviewEndpoint>
}

export type ImagePreviewCatalog = {
  models?: Record<string, ImagePreviewModel>
}

export type ImagePreviewSelection = {
  group: string
  model: string
  endpoint: string
  size: string
  quality: string
  response_format: string
  n: number
}

export type ImagePreviewOptions = {
  groups: string[]
  models: string[]
  endpoints: string[]
  sizes: string[]
  qualities: string[]
  responseFormats: string[]
}

export type ImagePreviewEndpointOptions = Pick<
  ImagePreviewOptions,
  'endpoints' | 'sizes' | 'qualities' | 'responseFormats'
>

function parseObject(raw: string): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(raw || '{}')
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      return value as Record<string, unknown>
    }
  } catch {
    // The JSON editor reports validation errors; preview falls back to empty options.
  }
  return {}
}

function asCatalog(raw: string): ImagePreviewCatalog {
  const value = parseObject(raw)
  const models = value.models
  if (!models || typeof models !== 'object' || Array.isArray(models)) {
    return { models: {} }
  }
  return { models: models as Record<string, ImagePreviewModel> }
}

function asGroups(raw: string): string[] {
  const value = parseObject(raw)
  const groups = value.groups
  const names =
    groups && typeof groups === 'object' && !Array.isArray(groups)
      ? Object.keys(groups as Record<string, unknown>)
      : []
  return ['default', ...names.filter((name) => name !== 'default')]
}

function stringOptions(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is string => typeof item === 'string' && item.trim() !== ''
      )
    : []
}

function endpointNames(model: ImagePreviewModel | undefined): string[] {
  const endpointMap = model?.endpoints ?? {}
  return Object.keys(endpointMap).filter(
    (endpoint) => endpointMap[endpoint]?.capability?.enabled !== false
  )
}

export function getImagePreviewEndpointOptions(
  catalogRaw: string,
  modelName: string,
  endpointName?: string
): ImagePreviewEndpointOptions {
  const catalog = asCatalog(catalogRaw)
  const model = catalog.models?.[modelName]
  const endpoints = endpointNames(model)
  const endpoint =
    endpointName && endpoints.includes(endpointName)
      ? endpointName
      : endpoints[0]
  const capability = endpoint
    ? model?.endpoints?.[endpoint]?.capability
    : undefined
  return {
    endpoints,
    sizes: stringOptions(capability?.sizes),
    qualities: stringOptions(capability?.qualities),
    responseFormats: stringOptions(capability?.response_formats),
  }
}

export function getImagePreviewOptions(
  catalogRaw: string,
  routingRaw: string
): ImagePreviewOptions {
  const catalog = asCatalog(catalogRaw)
  const models = catalog.models ?? {}
  const modelNames = Object.keys(models).sort()
  const firstModel = modelNames[0]
  const endpointMap = firstModel ? (models[firstModel]?.endpoints ?? {}) : {}
  const endpoints = endpointNames(models[firstModel])
  const firstEndpoint = endpoints[0]
  const capability = firstEndpoint
    ? endpointMap[firstEndpoint]?.capability
    : undefined

  return {
    groups: asGroups(routingRaw),
    models: modelNames,
    endpoints,
    sizes: stringOptions(capability?.sizes),
    qualities: stringOptions(capability?.qualities),
    responseFormats: stringOptions(capability?.response_formats),
  }
}

export function normalizeImagePreviewSelection(
  selection: ImagePreviewSelection,
  catalogRaw: string,
  routingRaw: string
): ImagePreviewSelection {
  const catalog = asCatalog(catalogRaw)
  const models = catalog.models ?? {}
  const modelNames = Object.keys(models).sort()
  const model = modelNames.includes(selection.model)
    ? selection.model
    : (modelNames[0] ?? '')
  const endpointMap = models[model]?.endpoints ?? {}
  const endpoints = Object.keys(endpointMap).filter(
    (endpoint) => endpointMap[endpoint]?.capability?.enabled !== false
  )
  const endpoint = endpoints.includes(selection.endpoint)
    ? selection.endpoint
    : (endpoints[0] ?? '')
  const capability = endpointMap[endpoint]?.capability
  const sizes = stringOptions(capability?.sizes)
  const qualities = stringOptions(capability?.qualities)
  const responseFormats = stringOptions(capability?.response_formats)
  const n =
    Number.isFinite(selection.n) && selection.n > 0
      ? Math.floor(selection.n)
      : 1

  return {
    group: asGroups(routingRaw).includes(selection.group)
      ? selection.group
      : 'default',
    model,
    endpoint,
    size: sizes.includes(selection.size) ? selection.size : (sizes[0] ?? ''),
    quality: qualities.includes(selection.quality)
      ? selection.quality
      : (qualities[0] ?? ''),
    response_format: responseFormats.includes(selection.response_format)
      ? selection.response_format
      : (responseFormats[0] ?? ''),
    n,
  }
}
