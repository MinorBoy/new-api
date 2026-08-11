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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_TYPES,
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES,
  MODEL_FETCHABLE_TYPES,
  TASK_ONLY_CHANNEL_TYPES,
} from '../../constants'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  getStatusOnChannelTypeChange,
} from '../channel-form'
import {
  getBaseUrlOnChannelTypeChange,
  getChannelModelOptions,
  getChannelTypeConfig,
  getChannelTypeHints,
} from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'

function newAPIForm(baseUrl: string) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'New API upstream',
    type: CHANNEL_TYPE_NEW_API,
    base_url: baseUrl,
    key: 'test-key',
    models: 'gpt-5',
  }
}

describe('New API channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_NEW_API
    )

    assert.deepEqual(option, {
      value: CHANNEL_TYPE_NEW_API,
      label: 'New API',
    })
    assert.equal(
      CHANNEL_TYPE_OPTIONS.findIndex(
        (item) => item.value === CHANNEL_TYPE_NEW_API
      ) + 1,
      CHANNEL_TYPE_OPTIONS.findIndex((item) => item.value === 58)
    )
    assert.equal(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_NEW_API), true)
    assert.equal(getChannelTypeIcon(CHANNEL_TYPE_NEW_API), 'NewAPI')
    assert.equal(
      getKeyPromptForType(CHANNEL_TYPE_NEW_API),
      'Enter API key for this channel'
    )
    assert.equal(getChannelTypeConfig(CHANNEL_TYPE_NEW_API).icon, 'NewAPI')
  })

  test('requires a non-blank Base URL', () => {
    const blankResult = channelFormSchema.safeParse(newAPIForm('  '))

    assert.equal(blankResult.success, false)
    if (!blankResult.success) {
      assert.equal(
        blankResult.error.issues.some(
          (issue) =>
            issue.path[0] === 'base_url' &&
            issue.message === 'Base URL is required for this channel type'
        ),
        true
      )
    }

    assert.equal(
      channelFormSchema.safeParse(newAPIForm('https://new-api.example'))
        .success,
      true
    )
  })

  test('keeps Sub2API Base URL validation unchanged', () => {
    const result = channelFormSchema.safeParse({
      ...newAPIForm(''),
      type: 59,
    })

    assert.equal(result.success, true)
  })
})

describe('pre-acceptance channel defaults', () => {
  test('selecting OmegaAI defaults a new channel to manually disabled', () => {
    assert.equal(
      getStatusOnChannelTypeChange(1, 208, CHANNEL_FORM_DEFAULT_VALUES.status),
      2
    )
  })

  test('switching an existing channel to OmegaAI disables it without overriding an explicit status', () => {
    assert.equal(getStatusOnChannelTypeChange(1, 208, 1), 2)
    assert.equal(getStatusOnChannelTypeChange(208, 208, 1), 1)
    assert.equal(getStatusOnChannelTypeChange(208, 208, 2), 2)
  })

  test('selecting 4stoken defaults a new channel to manually disabled', () => {
    assert.equal(
      getStatusOnChannelTypeChange(1, 209, CHANNEL_FORM_DEFAULT_VALUES.status),
      2
    )
  })

  test('selecting 8yes defaults a new channel to manually disabled', () => {
    assert.equal(
      getStatusOnChannelTypeChange(1, 210, CHANNEL_FORM_DEFAULT_VALUES.status),
      2
    )
  })

  test('selecting ZZone defaults a new channel to manually disabled', () => {
    assert.equal(
      getStatusOnChannelTypeChange(1, 212, CHANNEL_FORM_DEFAULT_VALUES.status),
      2
    )
  })

  test('selecting FYLink defaults a new channel to manually disabled', () => {
    assert.equal(
      getStatusOnChannelTypeChange(1, 214, CHANNEL_FORM_DEFAULT_VALUES.status),
      2
    )
  })
})

describe('ZZone channel configuration', () => {
  test('registers task-only type 212 without inventing a model catalog', () => {
    assert.equal(CHANNEL_TYPES[212], 'ZZone')
    assert.deepEqual(
      CHANNEL_TYPE_OPTIONS.find((item) => item.value === 212),
      { value: 212, label: 'ZZone' }
    )
    assert.equal(getChannelTypeIcon(212), 'NewAPI')
    assert.equal(TASK_ONLY_CHANNEL_TYPES.has(212), true)
    assert.equal(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(212), true)
    assert.equal(MODEL_FETCHABLE_TYPES.has(212), false)
    assert.deepEqual(getChannelTypeConfig(212), {
      id: 212,
      name: 'ZZone',
      icon: 'NewAPI',
      defaultBaseUrl: 'https://zzone.cc.cd',
      supportedModels: [],
      hints: {
        baseUrl: 'Default: https://zzone.cc.cd',
        key: 'Enter the raw API key issued by ZZone',
        models:
          'Map client-visible Ark model names to verified ZZone upstream models',
      },
    })
    assert.deepEqual(getChannelModelOptions(212, [], []), [])
    assert.equal(
      getBaseUrlOnChannelTypeChange(212, '', false),
      'https://zzone.cc.cd'
    )
    assert.equal(
      getBaseUrlOnChannelTypeChange(212, 'https://proxy.example.com', false),
      'https://proxy.example.com'
    )
    assert.deepEqual(getChannelTypeHints(212), {
      baseUrl: 'Default: https://zzone.cc.cd',
      key: 'Enter the raw API key issued by ZZone',
      models:
        'Map client-visible Ark model names to verified ZZone upstream models',
    })
    assert.equal(
      CHANNEL_TYPE_WARNINGS[212],
      'ZZone is task-only. Enable it only after real upstream contract acceptance.'
    )
  })

})
