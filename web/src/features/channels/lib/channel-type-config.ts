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
import { CHANNEL_TYPES } from '../constants'

// ============================================================================
// Channel Type Configuration
// ============================================================================

export interface ChannelTypeConfig {
  id: number
  name: string
  icon: string
  defaultBaseUrl?: string
  requiresOrganization?: boolean
  requiresRegion?: boolean
  supportedModels?: string[]
  hints?: {
    baseUrl?: string
    key?: string
    models?: string
    other?: string
  }
  validation?: {
    keyFormat?: RegExp
    keyMinLength?: number
  }
}

/**
 * Configuration for each channel type
 */
export const CHANNEL_TYPE_CONFIGS: Record<number, ChannelTypeConfig> = {
  1: {
    id: 1,
    name: CHANNEL_TYPES[1],
    icon: 'openai',
    defaultBaseUrl: 'https://api.openai.com',
    requiresOrganization: true,
    hints: {
      baseUrl: 'Default: https://api.openai.com',
      key: 'Format: sk-...',
      models: 'gpt-4,gpt-4-turbo,gpt-3.5-turbo',
    },
    validation: {
      keyFormat: /^sk-/,
      keyMinLength: 20,
    },
  },
  3: {
    id: 3,
    name: CHANNEL_TYPES[3],
    icon: 'azure',
    requiresRegion: true,
    hints: {
      baseUrl: 'Azure OpenAI Endpoint',
      key: 'Azure API Key',
      models: 'Deployment names',
    },
  },
  14: {
    id: 14,
    name: CHANNEL_TYPES[14],
    icon: 'anthropic',
    defaultBaseUrl: 'https://api.anthropic.com',
    hints: {
      key: 'Format: sk-ant-...',
      models: 'claude-3-opus,claude-3-sonnet,claude-3-haiku',
    },
  },
  24: {
    id: 24,
    name: CHANNEL_TYPES[24],
    icon: 'google',
    hints: {
      key: 'Google API Key',
      models: 'gemini-pro,gemini-pro-vision',
    },
  },
  41: {
    id: 41,
    name: CHANNEL_TYPES[41],
    icon: 'google',
    requiresRegion: true,
    hints: {
      key: 'Service account JSON or API key',
      models: 'gemini-pro,gemini-1.5-pro',
      other: 'Region config: {"default": "us-central1"}',
    },
  },
  43: {
    id: 43,
    name: CHANNEL_TYPES[43],
    icon: 'deepseek',
    defaultBaseUrl: 'https://api.deepseek.com',
    hints: {
      key: 'DeepSeek API Key',
      models: 'deepseek-chat,deepseek-coder',
    },
  },
  20: {
    id: 20,
    name: CHANNEL_TYPES[20],
    icon: 'openrouter',
    defaultBaseUrl: 'https://openrouter.ai/api',
    hints: {
      key: 'OpenRouter API Key',
      models: 'Use model IDs from OpenRouter',
    },
  },
  56: {
    id: 56,
    name: CHANNEL_TYPES[56],
    icon: 'replicate',
    defaultBaseUrl: 'https://api.replicate.com',
    hints: {
      key: 'Replicate API Token',
      models: 'Replicate model IDs',
      baseUrl: 'Default: https://api.replicate.com',
    },
  },
  58: {
    id: 58,
    name: CHANNEL_TYPES[58],
    icon: 'newapi',
    hints: {
      baseUrl: 'Fallback base URL',
      key: 'Used by route auth templates',
      models: 'Models exposed by this channel',
    },
  },
  59: {
    id: 59,
    name: CHANNEL_TYPES[59],
    icon: 'Sub2API',
    hints: {
      baseUrl: 'Sub2API gateway base URL',
      key: 'Sub2API API Key',
      models: 'Models fetched from upstream /v1/models',
    },
  },
  60: {
    id: 60,
    name: CHANNEL_TYPES[60],
    icon: 'NewAPI',
    hints: {
      baseUrl: 'Base URL is required for this channel type',
      key: 'Enter API key for this channel',
      models: 'Models',
    },
  },
  200: {
    id: 200,
    name: CHANNEL_TYPES[200],
    icon: 'Dimensio',
    defaultBaseUrl: 'https://jimeng.dimensio.cn',
    supportedModels: [
      'jimeng-video-seedance-2.0-fast-vip',
      'jimeng-video-seedance-2.0-mini',
      'jimeng-video-seedance-2.0-vip',
    ],
    hints: {
      baseUrl: 'Default: https://jimeng.dimensio.cn',
      key: 'Enter the raw API key issued by Dimensio',
      models:
        'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    },
  },
  201: {
    id: 201,
    name: CHANNEL_TYPES[201],
    icon: 'NewAPI',
    supportedModels: [],
    hints: {
      baseUrl: 'Enter the upstream NewAPI base URL',
      key: 'Enter the upstream NewAPI video API key',
      models: 'Add client model names and map them to upstream video models',
    },
  },
  202: {
    id: 202,
    name: CHANNEL_TYPES[202],
    icon: 'Jimeng',
    defaultBaseUrl: 'https://clmm-mall.top',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://clmm-mall.top',
      key: 'Enter the raw API key issued by CLMM Mall',
      models:
        'Use client-visible Ark model names and map them to complete CLMM Mall model names.',
    },
  },
  203: {
    id: 203,
    name: CHANNEL_TYPES[203],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://lucen.asia',
    supportedModels: [
      'seedance-480p-5s',
      'seedance-480p-10s',
      'seedance-480p-15s',
      'seedance-720p-5s',
      'seedance-720p-10s',
      'seedance-720p-15s',
      'seedance-1080p-5s',
      'seedance-1080p-10s',
      'seedance-1080p-15s',
      'seedance-480p-token',
      'seedance-720p-token',
      'seedance-1080p-token',
    ],
    hints: {
      baseUrl: 'Default: https://lucen.asia',
      key: 'Enter the API key issued by Lucen',
      models:
        "Select Lucen models matching this channel's fixed-duration or token-billing API key",
    },
  },
  204: {
    id: 204,
    name: CHANNEL_TYPES[204],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://newapi.megabyai.cc',
    supportedModels: ['videos-standard', 'videos-fast', 'videos-mini'],
    hints: {
      baseUrl: 'Default: https://newapi.megabyai.cc',
      key: 'Enter the raw API key issued by MegaByAI',
      models:
        'Supported upstream models: videos-standard, videos-fast, videos-mini',
    },
  },
  205: {
    id: 205,
    name: CHANNEL_TYPES[205],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://ai.cangyuansuanli.cn',
    supportedModels: ['seedance-2.0-720p'],
    hints: {
      baseUrl: 'Default: https://ai.cangyuansuanli.cn',
      key: 'Enter the raw API key issued by Cangyuan',
      models:
        'The documented initial model is seedance-2.0-720p; administrators may add verified models manually',
    },
  },
  206: {
    id: 206,
    name: CHANNEL_TYPES[206],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://api.paipu.net',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://api.paipu.net',
      key: 'Enter the raw API key issued by Paipu',
      models:
        'Import Paipu models from channel configuration or add verified mappings manually',
    },
  },
  207: {
    id: 207,
    name: CHANNEL_TYPES[207],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://token.secure-skill.com',
    supportedModels: ['video-2.0-fast', 'video-2.0-mini', 'video-2.0-pro'],
    hints: {
      baseUrl: 'Default: https://token.secure-skill.com',
      key: 'Enter the API key issued for the selected Secure video group',
      models: 'Select only models enabled for this Secure group API key',
    },
  },
  208: {
    id: 208,
    name: CHANNEL_TYPES[208],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://omegaai.xin',
    supportedModels: [
      'klsdpro2-720p',
      'seedance-v2-720p',
      'dola-seedance-2.0',
      'lingjing-video-v1',
      'db-ai-video-v1',
    ],
    hints: {
      baseUrl: 'Default: https://omegaai.xin',
      key: 'Enter the raw API key issued by OmegaAI',
      models: 'Select the verified OmegaAI Seedance video models',
    },
  },
  209: {
    id: 209,
    name: CHANNEL_TYPES[209],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://api.4stoken.cn',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://api.4stoken.cn',
      key: 'Enter the raw API key issued by 4stoken',
      models:
        'Map client-visible Ark model names to verified 4stoken upstream models',
    },
  },
  210: {
    id: 210,
    name: CHANNEL_TYPES[210],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://8yes.cc',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://8yes.cc',
      key: 'Enter the raw API key issued by 8yes',
      models:
        'Map client-visible Ark model names to verified 8yes upstream models',
    },
  },
  211: {
    id: 211,
    name: CHANNEL_TYPES[211],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://z5api.com',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://z5api.com',
      key: 'Enter the raw API key issued by Z5API',
      models:
        'Map client-visible Ark model names to verified Z5API upstream models',
    },
  },
  212: {
    id: 212,
    name: CHANNEL_TYPES[212],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://zzone.cc.cd',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://zzone.cc.cd',
      key: 'Enter the raw API key issued by ZZone',
      models:
        'Map client-visible Ark model names to verified ZZone upstream models',
    },
  },
  213: {
    id: 213,
    name: CHANNEL_TYPES[213],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://api.mikoto.vip',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://api.mikoto.vip',
      key: 'Enter the raw API key issued by Mikoto',
      models:
        'Map client-visible Ark model names to verified Mikoto upstream models',
    },
  },
  214: {
    id: 214,
    name: CHANNEL_TYPES[214],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://api.fflink.top',
    supportedModels: [],
    hints: {
      baseUrl: 'Default: https://api.fflink.top',
      key: 'Enter the raw API key issued by FYLink',
      models:
        'Map client-visible Ark model names to verified FYLink upstream models',
    },
  },
  215: {
    id: 215,
    name: CHANNEL_TYPES[215],
    icon: 'NewAPI',
    defaultBaseUrl: 'https://api.wxart.space',
    supportedModels: ['seedance2.0', 'seedance2.5'],
    hints: {
      baseUrl: 'Default: https://api.wxart.space',
      key: 'Enter the raw API key issued by WxArt',
      models: 'Map client-visible Ark model names to verified WxArt Seedance models',
    },
  },
}

const MANAGED_DEFAULT_BASE_URL_TYPES = new Set([
  200, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215,
])

const KNOWN_PROVIDER_BASE_URLS = new Set([
  ...Object.values(CHANNEL_TYPE_CONFIGS)
    .map((config) => config.defaultBaseUrl)
    .filter((baseUrl): baseUrl is string => Boolean(baseUrl)),
  'https://ark.cn-beijing.volces.com',
  'https://ark.ap-southeast.bytepluses.com',
])

/**
 * Get configuration for a channel type
 */
export function getChannelTypeConfig(type: number): ChannelTypeConfig {
  return (
    CHANNEL_TYPE_CONFIGS[type] || {
      id: type,
      name: CHANNEL_TYPES[type as keyof typeof CHANNEL_TYPES] || 'Unknown',
      icon: 'openai',
    }
  )
}

/**
 * Check if channel type requires organization field
 */
export function requiresOrganization(type: number): boolean {
  return CHANNEL_TYPE_CONFIGS[type]?.requiresOrganization || false
}

/**
 * Check if channel type requires region configuration
 */
export function requiresRegion(type: number): boolean {
  return CHANNEL_TYPE_CONFIGS[type]?.requiresRegion || false
}

/**
 * Get default base URL for channel type
 */
export function getDefaultBaseUrl(type: number): string {
  return CHANNEL_TYPE_CONFIGS[type]?.defaultBaseUrl || ''
}

export function getBaseUrlOnChannelTypeChange(
  type: number,
  currentBaseUrl: string,
  isDirty: boolean
): string {
  if (!MANAGED_DEFAULT_BASE_URL_TYPES.has(type)) return currentBaseUrl

  const defaultBaseUrl = getDefaultBaseUrl(type)
  if (isDirty || !defaultBaseUrl) return currentBaseUrl
  if (currentBaseUrl && !KNOWN_PROVIDER_BASE_URLS.has(currentBaseUrl)) {
    return currentBaseUrl
  }
  return defaultBaseUrl
}

/**
 * Get hints for channel type
 */
export function getChannelTypeHints(type: number) {
  return CHANNEL_TYPE_CONFIGS[type]?.hints || {}
}

export function getChannelModelOptions(
  type: number,
  availableModels: string[],
  selectedModels: string[]
): string[] {
  return [
    ...new Set([
      ...(getChannelTypeConfig(type).supportedModels ?? []),
      ...availableModels,
      ...selectedModels,
    ]),
  ]
}

/**
 * Validate API key format for channel type
 */
export function validateKeyFormat(type: number, key: string): boolean {
  const config = CHANNEL_TYPE_CONFIGS[type]
  if (!config?.validation) return true

  const { keyFormat, keyMinLength } = config.validation

  if (keyMinLength && key.length < keyMinLength) {
    return false
  }

  if (keyFormat && !keyFormat.test(key)) {
    return false
  }

  return true
}
