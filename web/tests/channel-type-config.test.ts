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
  test('registers type 200 in the standard channel options', () => {
    expect(CHANNEL_TYPES[200]).toBe('Dimensio')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 200,
      label: 'Dimensio',
    })
    expect(getChannelTypeIcon(200)).toBe('Dimensio')
  })

  test('provides the Dimensio form defaults and guidance', () => {
    expect(getChannelTypeConfig(200)).toMatchObject({
      id: 200,
      name: 'Dimensio',
      icon: 'Dimensio',
      defaultBaseUrl: 'https://jimeng.dimensio.cn',
      supportedModels: [
        'jimeng-video-seedance-2.0-fast-vip',
        'jimeng-video-seedance-2.0-mini',
        'jimeng-video-seedance-2.0-vip',
      ],
    })
    expect(getDefaultBaseUrl(200)).toBe('https://jimeng.dimensio.cn')
    expect(getChannelTypeHints(200)).toEqual({
      baseUrl: 'Default: https://jimeng.dimensio.cn',
      key: 'Enter the raw API key issued by Dimensio',
      models:
        'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    })
    expect(TYPE_TO_KEY_PROMPT[200]).toBe(
      'Enter the raw API key issued by Dimensio'
    )
    expect(CHANNEL_TYPE_WARNINGS[200]).toBe(
      'Dimensio is task-only. Call it through the ARK /api/v3 task API.'
    )
  })

  test('does not enable generic model fetching for Dimensio', () => {
    expect(MODEL_FETCHABLE_TYPES.has(200)).toBe(false)
  })

  test('replaces provider URLs but preserves administrator URLs', () => {
    expect(
      getBaseUrlOnChannelTypeChange(
        200,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(
        200,
        'https://ark.ap-southeast.bytepluses.com',
        false
      )
    ).toBe('https://jimeng.dimensio.cn')
    expect(
      getBaseUrlOnChannelTypeChange(200, 'https://proxy.example.com', true)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(200, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })

  test('disables generic channel testing for task-only Dimensio', () => {
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(200)).toBe(true)
  })
})

describe('NewAPIVideo channel configuration', () => {
  test('registers task-only type 201 without fake defaults', () => {
    expect(CHANNEL_TYPES[201]).toBe('NewAPIVideo')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 201,
      label: 'NewAPIVideo',
    })
    expect(getChannelTypeIcon(201)).toBe('NewAPI')
    expect(getDefaultBaseUrl(201)).toBe('')
    expect(getChannelTypeConfig(201).supportedModels).toEqual([])
    expect(MODEL_FETCHABLE_TYPES.has(201)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(201)).toBe(true)
  })

  test('provides protocol-specific guidance', () => {
    expect(TYPE_TO_KEY_PROMPT[201]).toBe(
      'Enter the upstream NewAPI video API key'
    )
    expect(CHANNEL_TYPE_WARNINGS[201]).toBe(
      'NewAPIVideo is task-only. Call it through /v1/video/generations or the ARK /api/v3 task API.'
    )
    expect(getChannelTypeHints(201)).toEqual({
      baseUrl: 'Enter the upstream NewAPI base URL',
      key: 'Enter the upstream NewAPI video API key',
      models: 'Add client model names and map them to upstream video models',
    })
  })
})

describe('CLMM Mall channel configuration', () => {
  test('registers type 202 in the standard channel options', () => {
    expect(CHANNEL_TYPES[202]).toBe('CLMM Mall')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 202,
      label: 'CLMM Mall',
    })
    expect(getChannelTypeIcon(202)).toBe('Jimeng')
  })

  test('provides the CLMM Mall form defaults and guidance', () => {
    expect(getChannelTypeConfig(202)).toMatchObject({
      id: 202,
      name: 'CLMM Mall',
      icon: 'Jimeng',
      defaultBaseUrl: 'https://clmm-mall.top',
      supportedModels: [],
    })
    expect(getDefaultBaseUrl(202)).toBe('https://clmm-mall.top')
    expect(getChannelTypeHints(202)).toEqual({
      baseUrl: 'Default: https://clmm-mall.top',
      key: 'Enter the raw API key issued by CLMM Mall',
      models:
        'Use client-visible Ark model names and map them to complete CLMM Mall model names.',
    })
    expect(TYPE_TO_KEY_PROMPT[202]).toBe(
      'Enter the raw API key issued by CLMM Mall'
    )
    expect(CHANNEL_TYPE_WARNINGS[202]).toBe(
      'CLMM Mall is task-only. Call it through the Ark /api/v3 task API.'
    )
    expect(MODEL_FETCHABLE_TYPES.has(202)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(202)).toBe(true)
  })

  test('replaces known defaults but preserves administrator URLs and dirty values', () => {
    expect(
      getBaseUrlOnChannelTypeChange(202, 'https://jimeng.dimensio.cn', false)
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(
        202,
        'https://ark.cn-beijing.volces.com',
        false
      )
    ).toBe('https://clmm-mall.top')
    expect(
      getBaseUrlOnChannelTypeChange(202, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
    expect(
      getBaseUrlOnChannelTypeChange(202, 'https://jimeng.dimensio.cn', true)
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
    expect(CHANNEL_TYPES[203]).toBe('Lucen')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 203, label: 'Lucen' })
    expect(getChannelTypeIcon(203)).toBe('NewAPI')
    expect(getDefaultBaseUrl(203)).toBe('https://lucen.asia')
    expect(getChannelTypeConfig(203).supportedModels).toEqual([
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
    expect(MODEL_FETCHABLE_TYPES.has(203)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(203)).toBe(true)
    expect(TASK_ONLY_CHANNEL_TYPES.has(203)).toBe(true)
  })

  test('explains the two ordinary API-key channels without a group field', () => {
    expect(TYPE_TO_KEY_PROMPT[203]).toBe('Enter the API key issued by Lucen')
    expect(CHANNEL_TYPE_WARNINGS[203]).toBe(
      'Lucen is task-only. Create separate channels for the fixed-duration key and token-billing key.'
    )
    expect(getChannelTypeHints(203)).toEqual({
      baseUrl: 'Default: https://lucen.asia',
      key: 'Enter the API key issued by Lucen',
      models:
        "Select Lucen models matching this channel's fixed-duration or token-billing API key",
    })
  })

  test('exposes configured Lucen models when the upstream catalog is empty', () => {
    expect(getChannelModelOptions(203, [], [])).toEqual([
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
  test('registers task-only type 204', () => {
    expect(CHANNEL_TYPES[204]).toBe('MegaByAI')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 204,
      label: 'MegaByAI',
    })
    expect(getChannelTypeIcon(204)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(204)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(204)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(204)).toBe(false)
  })

  test('provides defaults and models', () => {
    expect(getChannelTypeConfig(204)).toMatchObject({
      id: 204,
      name: 'MegaByAI',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://newapi.megabyai.cc',
      supportedModels: ['videos-standard', 'videos-fast', 'videos-mini'],
    })
    expect(getDefaultBaseUrl(204)).toBe('https://newapi.megabyai.cc')
    expect(getChannelTypeHints(204)).toEqual({
      baseUrl: 'Default: https://newapi.megabyai.cc',
      key: 'Enter the raw API key issued by MegaByAI',
      models:
        'Supported upstream models: videos-standard, videos-fast, videos-mini',
    })
    expect(TYPE_TO_KEY_PROMPT[204]).toBe(
      'Enter the raw API key issued by MegaByAI'
    )
    expect(CHANNEL_TYPE_WARNINGS[204]).toBe(
      'MegaByAI is task-only. Call it through the Ark /api/v3 task API.'
    )
  })

  test('exposes configured models when the upstream catalog is empty', () => {
    expect(getChannelModelOptions(204, [], [])).toEqual([
      'videos-standard',
      'videos-fast',
      'videos-mini',
    ])
  })
})

describe('Cangyuan channel configuration', () => {
  test('registers task-only type 205', () => {
    expect(CHANNEL_TYPES[205]).toBe('Cangyuan')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({
      value: 205,
      label: 'Cangyuan',
    })
    expect(getChannelTypeIcon(205)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(205)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(205)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(205)).toBe(false)
  })

  test('provides the documented default and initial model', () => {
    expect(getChannelTypeConfig(205)).toMatchObject({
      id: 205,
      name: 'Cangyuan',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://ai.cangyuansuanli.cn',
      supportedModels: ['seedance-2.0-720p'],
    })
    expect(getDefaultBaseUrl(205)).toBe('https://ai.cangyuansuanli.cn')
    expect(getChannelTypeHints(205)).toEqual({
      baseUrl: 'Default: https://ai.cangyuansuanli.cn',
      key: 'Enter the raw API key issued by Cangyuan',
      models:
        'The documented initial model is seedance-2.0-720p; administrators may add verified models manually',
    })
    expect(TYPE_TO_KEY_PROMPT[205]).toBe(
      'Enter the raw API key issued by Cangyuan'
    )
    expect(CHANNEL_TYPE_WARNINGS[205]).toBe(
      'Cangyuan is task-only. Call it through the Ark /api/v3 task API.'
    )
  })

  test('exposes the configured model and applies the managed default URL', () => {
    expect(getChannelModelOptions(205, [], [])).toEqual(['seedance-2.0-720p'])
    expect(
      getBaseUrlOnChannelTypeChange(205, 'https://newapi.megabyai.cc', false)
    ).toBe('https://ai.cangyuansuanli.cn')
    expect(
      getBaseUrlOnChannelTypeChange(205, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})

describe('Paipu channel configuration', () => {
  test('registers task-only type 206', () => {
    expect(CHANNEL_TYPES[206]).toBe('Paipu')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 206, label: 'Paipu' })
    expect(getChannelTypeIcon(206)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(206)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(206)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(206)).toBe(false)
  })

  test('provides the documented default and model directory', () => {
    expect(getChannelTypeConfig(206)).toMatchObject({
      id: 206,
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
    expect(getDefaultBaseUrl(206)).toBe('https://api.paipu.net')
    expect(getChannelTypeHints(206)).toEqual({
      baseUrl: 'Default: https://api.paipu.net',
      key: 'Enter the raw API key issued by Paipu',
      models: 'Select from the 24 documented Paipu /v1/videos models',
    })
    expect(TYPE_TO_KEY_PROMPT[206]).toBe(
      'Enter the raw API key issued by Paipu'
    )
    expect(CHANNEL_TYPE_WARNINGS[206]).toBe(
      'Paipu is task-only. Enable it only after real upstream contract acceptance.'
    )
  })

  test('exposes the configured models and applies the managed default URL', () => {
    expect(getChannelModelOptions(206, [], [])).toHaveLength(24)
    expect(
      getBaseUrlOnChannelTypeChange(206, 'https://newapi.megabyai.cc', false)
    ).toBe('https://api.paipu.net')
    expect(
      getBaseUrlOnChannelTypeChange(206, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})

describe('Secure channel configuration', () => {
  test('registers task-only type 207', () => {
    expect(CHANNEL_TYPES[207]).toBe('Secure')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 207, label: 'Secure' })
    expect(getChannelTypeIcon(207)).toBe('NewAPI')
    expect(TASK_ONLY_CHANNEL_TYPES.has(207)).toBe(true)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(207)).toBe(true)
    expect(MODEL_FETCHABLE_TYPES.has(207)).toBe(false)
  })

  test('uses the Secure default, models, and group-specific key guidance', () => {
    expect(getChannelTypeConfig(207)).toMatchObject({
      id: 207,
      name: 'Secure',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://token.secure-skill.com',
      supportedModels: ['video-2.0-fast', 'video-2.0-mini', 'video-2.0-pro'],
    })
    expect(getDefaultBaseUrl(207)).toBe('https://token.secure-skill.com')
    expect(getChannelTypeHints(207)).toEqual({
      baseUrl: 'Default: https://token.secure-skill.com',
      key: 'Enter the API key issued for the selected Secure video group',
      models: 'Select only models enabled for this Secure group API key',
    })
    expect(TYPE_TO_KEY_PROMPT[207]).toBe(
      'Enter the API key issued for the selected Secure video group'
    )
    expect(CHANNEL_TYPE_WARNINGS[207]).toBe(
      'Secure is task-only. Create separate channels for the Discount, Overseas, and Enterprise keys.'
    )
  })

  test('applies the managed Secure default URL and exposes the model list', () => {
    expect(getChannelModelOptions(207, [], [])).toEqual([
      'video-2.0-fast',
      'video-2.0-mini',
      'video-2.0-pro',
    ])
    expect(
      getBaseUrlOnChannelTypeChange(207, 'https://api.paipu.net', false)
    ).toBe('https://token.secure-skill.com')
    expect(
      getBaseUrlOnChannelTypeChange(207, 'https://proxy.example.com', false)
    ).toBe('https://proxy.example.com')
  })
})
