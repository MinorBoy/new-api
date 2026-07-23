import { describe, expect, test } from 'bun:test'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  CHANNEL_TYPES,
  GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES,
  MODEL_FETCHABLE_TYPES,
  TYPE_TO_KEY_PROMPT,
} from '../src/features/channels/constants'
import {
  getBaseUrlOnChannelTypeChange,
  getChannelTypeConfig,
  getChannelTypeHints,
  getDefaultBaseUrl,
} from '../src/features/channels/lib/channel-type-config'
import { getChannelTypeIcon } from '../src/features/channels/lib/channel-utils'

describe('Dimensio channel configuration', () => {
  test('registers type 59 in the standard channel options', () => {
    expect(CHANNEL_TYPES[59]).toBe('Dimensio')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 59,
      label: 'Dimensio',
    })
    expect(getChannelTypeIcon(59)).toBe('Dimensio')
  })

  test('provides the Dimensio form defaults and guidance', () => {
    expect(getChannelTypeConfig(59)).toMatchObject({
      id: 59,
      name: 'Dimensio',
      icon: 'Dimensio',
      defaultBaseUrl: 'https://jimeng.dimensio.cn',
      supportedModels: [
        'jimeng-video-seedance-2.0-fast-vip',
        'jimeng-video-seedance-2.0-mini',
        'jimeng-video-seedance-2.0-vip',
      ],
    })
    expect(getDefaultBaseUrl(59)).toBe('https://jimeng.dimensio.cn')
    expect(getChannelTypeHints(59)).toEqual({
      baseUrl: 'Default: https://jimeng.dimensio.cn',
      key: 'Enter the raw API key issued by Dimensio',
      models:
        'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    })
    expect(TYPE_TO_KEY_PROMPT[59]).toBe(
      'Enter the raw API key issued by Dimensio'
    )
    expect(CHANNEL_TYPE_WARNINGS[59]).toBe(
      'Dimensio is task-only. Call it through the ARK /api/v3 task API.'
    )
  })

  test('does not enable generic model fetching for Dimensio', () => {
    expect(MODEL_FETCHABLE_TYPES.has(59)).toBe(false)
  })

  test('replaces provider URLs but preserves administrator URLs', () => {
    expect(
      getBaseUrlOnChannelTypeChange(
        59,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(
        59,
        'https://ark.ap-southeast.bytepluses.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(59, 'https://proxy.example.com', true)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(59, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })

  test('disables generic channel testing for task-only Dimensio', () => {
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(59)).toBe(true)
  })
})

describe('NewAPIVideo channel configuration', () => {
  test('registers task-only type 60 without fake defaults', () => {
    expect(CHANNEL_TYPES[60]).toBe('NewAPIVideo')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 60,
      label: 'NewAPIVideo',
    })
    expect(getChannelTypeIcon(60)).toBe('NewAPI')
    expect(getDefaultBaseUrl(60)).toBe('')
    expect(getChannelTypeConfig(60).supportedModels).toEqual([])
    expect(MODEL_FETCHABLE_TYPES.has(60)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(60)).toBe(true)
  })

  test('provides protocol-specific guidance', () => {
    expect(TYPE_TO_KEY_PROMPT[60]).toBe(
      'Enter the upstream NewAPI video API key'
    )
    expect(CHANNEL_TYPE_WARNINGS[60]).toBe(
      'NewAPIVideo is task-only. Call it through /v1/video/generations or the ARK /api/v3 task API.'
    )
    expect(getChannelTypeHints(60)).toEqual({
      baseUrl: 'Enter the upstream NewAPI base URL',
      key: 'Enter the upstream NewAPI video API key',
      models: 'Add client model names and map them to upstream video models',
    })
  })
})

describe('CLMM Mall channel configuration', () => {
  test('registers type 61 in the standard channel options', () => {
    expect(CHANNEL_TYPES[61]).toBe('CLMM Mall')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 61,
      label: 'CLMM Mall',
    })
    expect(getChannelTypeIcon(61)).toBe('Jimeng')
  })

  test('provides the CLMM Mall form defaults and guidance', () => {
    expect(getChannelTypeConfig(61)).toMatchObject({
      id: 61,
      name: 'CLMM Mall',
      icon: 'Jimeng',
      defaultBaseUrl: 'https://clmm-mall.top',
      supportedModels: [],
    })
    expect(getDefaultBaseUrl(61)).toBe('https://clmm-mall.top')
    expect(getChannelTypeHints(61)).toEqual({
      baseUrl: 'Default: https://clmm-mall.top',
      key: 'Enter the raw API key issued by CLMM Mall',
      models:
        'Use client-visible Ark model names and map them to complete CLMM Mall model names.',
    })
    expect(TYPE_TO_KEY_PROMPT[61]).toBe(
      'Enter the raw API key issued by CLMM Mall'
    )
    expect(CHANNEL_TYPE_WARNINGS[61]).toBe(
      'CLMM Mall is task-only. Call it through the Ark /api/v3 task API.'
    )
    expect(MODEL_FETCHABLE_TYPES.has(61)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(61)).toBe(true)
  })

  test('replaces known defaults but preserves administrator URLs and dirty values', () => {
    expect(
      getBaseUrlOnChannelTypeChange(61, 'https://jimeng.dimensio.cn', false)
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(
        61,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(61, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(61, 'https://jimeng.dimensio.cn', true)
    ).toBe('https://jimeng.dimensio.cn')
  })
})

describe('Channel base URL transition policy', () => {
  test('does not auto-fill configured defaults for unmanaged providers', () => {
    expect(getBaseUrlOnChannelTypeChange(1, '', false)).toBe('')
  })
})
