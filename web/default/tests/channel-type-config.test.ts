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
