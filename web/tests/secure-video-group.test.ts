import { describe, expect, test } from 'bun:test'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from '../src/features/channels/lib/channel-form'
import { shouldShowSecureVideoGroup } from '../src/features/channels/lib/secure-video-group'

describe('Secure video group form behavior', () => {
  test('shows the group field only for Secure', () => {
    expect(shouldShowSecureVideoGroup(66)).toBe(true)
    expect(shouldShowSecureVideoGroup(65)).toBe(false)
    expect(shouldShowSecureVideoGroup(1)).toBe(false)
  })

  test('requires a group for Secure', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 66,
      name: 'Secure',
      key: 'secret',
      models: 'video-2.0-pro',
    })
    expect(result.success).toBe(false)
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({ path: ['secure_video_group'] })
    )
  })

  test('persists the selected group and removes it after changing type', () => {
    const secure = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 66,
      name: 'Secure enterprise',
      key: 'secret',
      models: 'video-2.0-pro',
      secure_video_group: 'enterprise',
    })
    expect(
      JSON.parse(secure.channel.settings as string).secure_video_group
    ).toBe('enterprise')

    const other = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 1,
      name: 'OpenAI',
      key: 'secret',
      models: 'gpt-4o',
      settings: secure.channel.settings,
      secure_video_group: undefined,
    })
    expect(JSON.parse(other.channel.settings as string)).not.toHaveProperty(
      'secure_video_group'
    )
  })
})
