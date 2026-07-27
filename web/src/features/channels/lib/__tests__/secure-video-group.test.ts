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
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from '../channel-form'
import * as secureVideoGroup from '../secure-video-group'

type AddModeOption = { value: string; label: string }

describe('Secure video group form behavior', () => {
  test('shows the group field only for Secure', () => {
    assert.equal(secureVideoGroup.shouldShowSecureVideoGroup(207), true)
    assert.equal(secureVideoGroup.shouldShowSecureVideoGroup(206), false)
    assert.equal(secureVideoGroup.shouldShowSecureVideoGroup(1), false)
  })

  test('requires a group for Secure', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 207,
      name: 'Secure',
      key: 'secret',
      models: 'video-2.0-pro',
    })
    assert.equal(result.success, false)
    assert.ok(
      result.error?.issues.some(
        (issue) => issue.path[0] === 'secure_video_group'
      )
    )
  })

  test('rejects multi-key-to-single mode for Secure', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 207,
      name: 'Secure',
      key: 'key-one\nkey-two',
      models: 'video-2.0-pro',
      secure_video_group: 'discount',
      multi_key_mode: 'multi_to_single',
    })

    assert.equal(result.success, false)
    assert.ok(
      result.error?.issues.some((issue) => issue.path[0] === 'multi_key_mode')
    )
  })

  test('removes multi-key-to-single from the Secure add-mode control', () => {
    const filterSecureAddModeOptions = (
      secureVideoGroup as Record<string, unknown>
    ).filterSecureAddModeOptions as
      | ((
          type: number,
          options: readonly AddModeOption[]
        ) => readonly AddModeOption[])
      | undefined
    const options = [
      { value: 'single', label: 'Single' },
      { value: 'batch', label: 'Batch' },
      { value: 'multi_to_single', label: 'Multi-key' },
    ]

    assert.equal(typeof filterSecureAddModeOptions, 'function')
    assert.ok(filterSecureAddModeOptions)
    assert.deepEqual(
      filterSecureAddModeOptions(207, options).map((option) => option.value),
      ['single', 'batch']
    )
    assert.equal(filterSecureAddModeOptions(1, options), options)
  })

  test('locks type and group controls when editing an existing Secure channel', () => {
    const shouldLockSecureVideoIdentity = (
      secureVideoGroup as Record<string, unknown>
    ).shouldLockSecureVideoIdentity as
      | ((isEditing: boolean, originalType?: number) => boolean)
      | undefined

    assert.equal(typeof shouldLockSecureVideoIdentity, 'function')
    assert.ok(shouldLockSecureVideoIdentity)
    assert.equal(shouldLockSecureVideoIdentity(true, 207), true)
    assert.equal(shouldLockSecureVideoIdentity(false, 207), false)
    assert.equal(shouldLockSecureVideoIdentity(true, 206), false)
  })

  test('persists the selected group and removes it after changing type', () => {
    const secure = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 207,
      name: 'Secure enterprise',
      key: 'secret',
      models: 'video-2.0-pro',
      secure_video_group: 'enterprise',
    })
    assert.equal(
      JSON.parse(secure.channel.settings as string).secure_video_group,
      'enterprise'
    )

    const other = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 1,
      name: 'OpenAI',
      key: 'secret',
      models: 'gpt-4o',
      settings: secure.channel.settings,
      secure_video_group: undefined,
    })
    assert.equal(
      Object.hasOwn(
        JSON.parse(other.channel.settings as string),
        'secure_video_group'
      ),
      false
    )
  })
})
