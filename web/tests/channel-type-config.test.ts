import { describe, expect, test } from 'bun:test'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  CHANNEL_TYPES,
  GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES,
  MODEL_FETCHABLE_TYPES,
  TASK_ONLY_CHANNEL_TYPES,
  TYPE_TO_KEY_PROMPT,
} from '../src/features/channels/constants'
import {
  getBaseUrlOnChannelTypeChange,
  getChannelModelOptions,
  getChannelTypeConfig,
  getChannelTypeHints,
  getDefaultBaseUrl,
} from '../src/features/channels/lib/channel-type-config'
import { getChannelTypeIcon } from '../src/features/channels/lib/channel-utils'

describe('Dimensio channel configuration', () => {
  test('registers type 61 in the standard channel options', () => {
    expect(CHANNEL_TYPES[61]).toBe('Dimensio')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 61,
      label: 'Dimensio',
    })
    expect(getChannelTypeIcon(61)).toBe('Dimensio')
  })

  test('provides the Dimensio form defaults and guidance', () => {
    expect(getChannelTypeConfig(61)).toMatchObject({
      id: 61,
      name: 'Dimensio',
      icon: 'Dimensio',
      defaultBaseUrl: 'https://jimeng.dimensio.cn',
      supportedModels: [
        'jimeng-video-seedance-2.0-fast-vip',
        'jimeng-video-seedance-2.0-mini',
        'jimeng-video-seedance-2.0-vip',
      ],
    })
    expect(getDefaultBaseUrl(61)).toBe('https://jimeng.dimensio.cn')
    expect(getChannelTypeHints(61)).toEqual({
      baseUrl: 'Default: https://jimeng.dimensio.cn',
      key: 'Enter the raw API key issued by Dimensio',
      models:
        'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    })
    expect(TYPE_TO_KEY_PROMPT[61]).toBe(
      'Enter the raw API key issued by Dimensio'
    )
    expect(CHANNEL_TYPE_WARNINGS[61]).toBe(
      'Dimensio is task-only. Call it through the ARK /api/v3 task API.'
    )
  })

  test('does not enable generic model fetching for Dimensio', () => {
    expect(MODEL_FETCHABLE_TYPES.has(61)).toBe(false)
  })

  test('replaces provider URLs but preserves administrator URLs', () => {
    expect(
      getBaseUrlOnChannelTypeChange(
        61,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(
        61,
        'https://ark.ap-southeast.bytepluses.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(61, 'https://proxy.example.com', true)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(61, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })

  test('disables generic channel testing for task-only Dimensio', () => {
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(61)).toBe(true)
  })
})

describe('NewAPIVideo channel configuration', () => {
  test('registers task-only type 62 without fake defaults', () => {
    expect(CHANNEL_TYPES[62]).toBe('NewAPIVideo')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 62,
      label: 'NewAPIVideo',
    })
    expect(getChannelTypeIcon(62)).toBe('NewAPI')
    expect(getDefaultBaseUrl(62)).toBe('')
    expect(getChannelTypeConfig(62).supportedModels).toEqual([])
    expect(MODEL_FETCHABLE_TYPES.has(62)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(62)).toBe(true)
  })

  test('provides protocol-specific guidance', () => {
    expect(TYPE_TO_KEY_PROMPT[62]).toBe(
      'Enter the upstream NewAPI video API key'
    )
    expect(CHANNEL_TYPE_WARNINGS[62]).toBe(
      'NewAPIVideo is task-only. Call it through /v1/video/generations or the ARK /api/v3 task API.'
    )
    expect(getChannelTypeHints(62)).toEqual({
      baseUrl: 'Enter the upstream NewAPI base URL',
      key: 'Enter the upstream NewAPI video API key',
      models: 'Add client model names and map them to upstream video models',
    })
  })
})

describe('CLMM Mall channel configuration', () => {
  test('registers type 63 in the standard channel options', () => {
    expect(CHANNEL_TYPES[63]).toBe('CLMM Mall')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 63,
      label: 'CLMM Mall',
    })
    expect(getChannelTypeIcon(63)).toBe('Jimeng')
  })

  test('provides the CLMM Mall form defaults and guidance', () => {
    expect(getChannelTypeConfig(63)).toMatchObject({
      id: 63,
      name: 'CLMM Mall',
      icon: 'Jimeng',
      defaultBaseUrl: 'https://clmm-mall.top',
      supportedModels: [],
    })
    expect(getDefaultBaseUrl(63)).toBe('https://clmm-mall.top')
    expect(getChannelTypeHints(63)).toEqual({
      baseUrl: 'Default: https://clmm-mall.top',
      key: 'Enter the raw API key issued by CLMM Mall',
      models:
        'Use client-visible Ark model names and map them to complete CLMM Mall model names.',
    })
    expect(TYPE_TO_KEY_PROMPT[63]).toBe(
      'Enter the raw API key issued by CLMM Mall'
    )
    expect(CHANNEL_TYPE_WARNINGS[63]).toBe(
      'CLMM Mall is task-only. Call it through the Ark /api/v3 task API.'
    )
    expect(MODEL_FETCHABLE_TYPES.has(63)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(63)).toBe(true)
  })

  test('replaces known defaults but preserves administrator URLs and dirty values', () => {
    expect(
      getBaseUrlOnChannelTypeChange(63, 'https://jimeng.dimensio.cn', false)
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(
        63,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(63, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(63, 'https://jimeng.dimensio.cn', true)
    ).toBe('https://jimeng.dimensio.cn')
  })
})

describe('Channel base URL transition policy', () => {
  test('does not auto-fill configured defaults for unmanaged providers', () => {
    expect(getBaseUrlOnChannelTypeChange(1, '', false)).toBe('')
  })
})

describe('Lucen channel configuration', () => {
  test('uses one ordinary task-only channel type with all Lucen models', () => {
    expect(CHANNEL_TYPES[64]).toBe('Lucen')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 64, label: 'Lucen' })
    expect(getChannelTypeIcon(64)).toBe('NewAPI')
    expect(getDefaultBaseUrl(64)).toBe('https://lucen.asia')
    expect(getChannelTypeConfig(64).supportedModels).toEqual([
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
    ])
    expect(MODEL_FETCHABLE_TYPES.has(64)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(64)).toBe(true)
    expect(TASK_ONLY_CHANNEL_TYPES.has(64)).toBe(true)
  })

  test('explains the two ordinary API-key channels without a group field', () => {
    expect(TYPE_TO_KEY_PROMPT[64]).toBe('Enter the API key issued by Lucen')
    expect(CHANNEL_TYPE_WARNINGS[64]).toBe(
      'Lucen is task-only. Create separate channels for the fixed-duration key and token-billing key.'
    )
    expect(getChannelTypeHints(64)).toEqual({
      baseUrl: 'Default: https://lucen.asia',
      key: 'Enter the API key issued by Lucen',
      models:
        "Select Lucen models matching this channel's fixed-duration or token-billing API key",
    })
  })

  test('exposes configured Lucen models when the upstream catalog is empty', () => {
    expect(getChannelModelOptions(64, [], [])).toEqual([
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
    ])
  })
})

describe('MegaByAI channel configuration', () => {
  test('registers task-only type 65', () => {
    expect(CHANNEL_TYPES[65]).toBe('MegaByAI')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 65,
      label: 'MegaByAI',
    })
    expect(getChannelTypeIcon(65)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(65)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(65)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(65)).toBe(false)
  })

  test('provides defaults and models', () => {
    expect(getChannelTypeConfig(65)).toMatchObject({
      id: 65,
      name: 'MegaByAI',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://newapi.megabyai.cc',
      supportedModels: ['videos-standard', 'videos-fast', 'videos-mini'],
    })
    expect(getDefaultBaseUrl(65)).toBe('https://newapi.megabyai.cc')
    expect(getChannelTypeHints(65)).toEqual({
      baseUrl: 'Default: https://newapi.megabyai.cc',
      key: 'Enter the raw API key issued by MegaByAI',
      models:
        'Supported upstream models: videos-standard, videos-fast, videos-mini',
    })
    expect(TYPE_TO_KEY_PROMPT[65]).toBe(
      'Enter the raw API key issued by MegaByAI'
    )
    expect(CHANNEL_TYPE_WARNINGS[65]).toBe(
      'MegaByAI is task-only. Call it through the Ark /api/v3 task API.'
    )
  })

  test('exposes configured models when the upstream catalog is empty', () => {
    expect(getChannelModelOptions(65, [], [])).toEqual([
      'videos-standard',
      'videos-fast',
      'videos-mini',
    ])
  })
})

describe('Cangyuan channel configuration', () => {
  test('registers task-only type 66', () => {
    expect(CHANNEL_TYPES[66]).toBe('Cangyuan')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 66,
      label: 'Cangyuan',
    })
    expect(getChannelTypeIcon(66)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(66)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(66)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(66)).toBe(false)
  })

  test('provides the documented default and initial model', () => {
    expect(getChannelTypeConfig(66)).toMatchObject({
      id: 66,
      name: 'Cangyuan',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://ai.cangyuansuanli.cn',
      supportedModels: ['seedance-2.0-720p'],
    })
    expect(getDefaultBaseUrl(66)).toBe('https://ai.cangyuansuanli.cn')
    expect(getChannelTypeHints(66)).toEqual({
      baseUrl: 'Default: https://ai.cangyuansuanli.cn',
      key: 'Enter the raw API key issued by Cangyuan',
      models:
        'The documented initial model is seedance-2.0-720p; administrators may add verified models manually',
    })
    expect(TYPE_TO_KEY_PROMPT[66]).toBe(
      'Enter the raw API key issued by Cangyuan'
    )
    expect(CHANNEL_TYPE_WARNINGS[66]).toBe(
      'Cangyuan is task-only. Call it through the Ark /api/v3 task API.'
    )
  })

  test('exposes the configured model and applies the managed default URL', () => {
    expect(getChannelModelOptions(66, [], [])).toEqual(['seedance-2.0-720p'])
    expect(
      getBaseUrlOnChannelTypeChange(66, 'https://newapi.megabyai.cc', false)
    ).toBe('https://ai.cangyuansuanli.cn')
    expect(
      getBaseUrlOnChannelTypeChange(66, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})

describe('Paipu channel configuration', () => {
  test('registers task-only type 67', () => {
    expect(CHANNEL_TYPES[67]).toBe('Paipu')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 67, label: 'Paipu' })
    expect(getChannelTypeIcon(67)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(67)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(67)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(67)).toBe(false)
  })

  test('provides the documented default and model directory', () => {
    expect(getChannelTypeConfig(67)).toMatchObject({
      id: 67,
      name: 'Paipu',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://api.paipu.net',
      supportedModels: [
        'lec-sz-seedance-2-0-480p',
        'lec-gongteng-seedance-2-0-720p',
        'lec-gongteng-seedance-2-0-fast-720p',
        'lec-gongteng-seedance-2-0-1080p',
        'lec-seedance-2-0',
        'lec-feituo-seedance-2-0-hn-fast-720p',
        'lec-feituo-seedance-2-0-hn-720p',
        'lec-feituo-seedance-2-0-xh-fast-933-720p',
        'lec-feituo-seedance-2-0-xh-pro-933-720p',
        'lec-feituo-seedance-2-0-ld-cvk-2',
        'lec-feituo-seedance-2-0-limited-720p',
        'lec-feituo-seedance-2-0-my-fast-upscaled-1080p',
        'lec-feituo-seedance-2-0-my-upscaled-1080p',
        'lec-seedance-videos-standard',
        'lec-seedance-videos-face-standard',
        'lec-seedance-videos-face-fast',
        'lec-seedance-videos-stable',
        'lec-seedance-videos-stable-fast',
        'lec-seedance-videos-stable-mini',
        'lec-seedance-videos-stable-720p',
        'lec-seedance-videos-fast-720p',
        'lec-seedance-videos-mini-720p',
        'lec-seedance-videos-fast',
        'lec-seedance-videos-mini',
      ],
    })
    expect(getDefaultBaseUrl(67)).toBe('https://api.paipu.net')
    expect(getChannelTypeHints(67)).toEqual({
      baseUrl: 'Default: https://api.paipu.net',
      key: 'Enter the raw API key issued by Paipu',
      models: 'Select from the 24 documented Paipu /v1/videos models',
    })
    expect(TYPE_TO_KEY_PROMPT[67]).toBe('Enter the raw API key issued by Paipu')
    expect(CHANNEL_TYPE_WARNINGS[67]).toBe(
      'Paipu is task-only. Enable it only after real upstream contract acceptance.'
    )
  })

  test('exposes the configured models and applies the managed default URL', () => {
    expect(getChannelModelOptions(67, [], [])).toHaveLength(24)
    expect(
      getBaseUrlOnChannelTypeChange(67, 'https://newapi.megabyai.cc', false)
    ).toBe('https://api.paipu.net')
    expect(
      getBaseUrlOnChannelTypeChange(67, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})

describe('Secure channel configuration', () => {
  test('registers task-only type 68', () => {
    expect(CHANNEL_TYPES[68]).toBe('Secure')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 68, label: 'Secure' })
    expect(getChannelTypeIcon(68)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(68)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(68)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(68)).toBe(false)
  })

  test('uses the Secure default, models, and group-specific key guidance', () => {
    expect(getChannelTypeConfig(68)).toMatchObject({
      id: 68,
      name: 'Secure',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://token.secure-skill.com',
      supportedModels: ['video-2.0-fast', 'video-2.0-mini', 'video-2.0-pro'],
    })
    expect(getDefaultBaseUrl(68)).toBe('https://token.secure-skill.com')
    expect(getChannelTypeHints(68)).toEqual({
      baseUrl: 'Default: https://token.secure-skill.com',
      key: 'Enter the API key issued for the selected Secure video group',
      models: 'Select only models enabled for this Secure group API key',
    })
    expect(TYPE_TO_KEY_PROMPT[68]).toBe(
      'Enter the API key issued for the selected Secure video group'
    )
    expect(CHANNEL_TYPE_WARNINGS[68]).toBe(
      'Secure is task-only. Create separate channels for the Discount, Overseas, and Enterprise keys.'
    )
  })

  test('applies the managed Secure default URL and exposes the model list', () => {
    expect(getChannelModelOptions(68, [], [])).toEqual([
      'video-2.0-fast',
      'video-2.0-mini',
      'video-2.0-pro',
    ])
    expect(
      getBaseUrlOnChannelTypeChange(68, 'https://api.paipu.net', false)
    ).toBe('https://token.secure-skill.com')
    expect(
      getBaseUrlOnChannelTypeChange(68, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})
