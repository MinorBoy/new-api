import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { t } from 'i18next'

import { DEFAULT_VIDEO_FORM } from '../lib/defaults'
import * as requestHelpers from '../lib/request'
import {
  buildVideoTaskCurl,
  buildVideoRequest,
  filterModelsForApiKey,
  validateMediaLimits,
} from '../lib/request'
import { createVideoGenerationSchema } from '../lib/schema'
import type { VideoMedia } from '../types'

describe('video generation request helpers', () => {
  test('builds the Seedance 2.0 default multimodal request', () => {
    const request = buildVideoRequest({
      model: 'doubao-seedance-2-0-260128',
      prompt: 'test prompt',
      imageSource: 'url',
      assetIds: [],
      media: {
        images: ['https://example.com/one.jpg'],
        videos: ['https://example.com/reference.mp4'],
        audios: ['https://example.com/music.mp3'],
      },
      resolution: '720p',
      ratio: '16:9',
      duration: 11,
      generateAudio: true,
      watermark: false,
      returnLastFrame: false,
    })

    assert.deepEqual(request.content, [
      { type: 'text', text: 'test prompt' },
      {
        type: 'image_url',
        image_url: { url: 'https://example.com/one.jpg' },
        role: 'reference_image',
      },
      {
        type: 'video_url',
        video_url: { url: 'https://example.com/reference.mp4' },
        role: 'reference_video',
      },
      {
        type: 'audio_url',
        audio_url: { url: 'https://example.com/music.mp3' },
        role: 'reference_audio',
      },
    ])
    assert.equal(request.model, 'doubao-seedance-2-0-260128')
    assert.equal(request.generate_audio, true)
    assert.equal(request.watermark, undefined)
  })

  test('omits disabled optional ARK features', () => {
    const request = buildVideoRequest({
      model: 'doubao-seedance-2-0-mini-260128',
      prompt: 'text-only acceptance test',
      imageSource: 'url',
      assetIds: [],
      media: { images: [], videos: [], audios: [] },
      resolution: '720p',
      ratio: '16:9',
      duration: 5,
      generateAudio: false,
      watermark: false,
      returnLastFrame: false,
    })

    assert.equal('generate_audio' in request, false)
    assert.equal('watermark' in request, false)
  })

  test('builds Ark asset references without conflicting image or video URLs', () => {
    const request = buildVideoRequest({
      ...DEFAULT_VIDEO_FORM,
      imageSource: 'asset',
      assetIds: ['asset-20260401123823-6d4x2', 'asset-20260401124109-k8p7m'],
    })

    assert.deepEqual(
      request.content.filter((item) => item.type === 'image_url'),
      [
        {
          type: 'image_url',
          role: 'reference_image',
          image_url: { url: 'asset://asset-20260401123823-6d4x2' },
        },
        {
          type: 'image_url',
          role: 'reference_image',
          image_url: { url: 'asset://asset-20260401124109-k8p7m' },
        },
      ]
    )
    assert.equal(
      request.content.some((item) => item.type === 'video_url'),
      false
    )
    assert.equal(
      request.content.some((item) => item.type === 'audio_url'),
      true
    )
  })

  test('uses model-specific reference media limits', () => {
    const getVideoMediaLimits = Reflect.get(
      requestHelpers,
      'getVideoMediaLimits'
    ) as ((model: string) => Record<keyof VideoMedia, number>) | undefined

    assert.equal(typeof getVideoMediaLimits, 'function')
    assert.deepEqual(getVideoMediaLimits?.('doubao-seedance-2-0-260128'), {
      images: 9,
      videos: 3,
      audios: 3,
    })
    assert.deepEqual(getVideoMediaLimits?.('doubao-seedance-2-5-260628'), {
      images: 30,
      videos: 10,
      audios: 10,
    })
    assert.deepEqual(getVideoMediaLimits?.('other-model'), {
      images: 9,
      videos: 3,
      audios: 3,
    })
  })

  test('only the base Seedance 2.0 model supports role assets', () => {
    const supportsRoleAssets = Reflect.get(
      requestHelpers,
      'supportsRoleAssets'
    ) as ((model: string) => boolean) | undefined

    assert.equal(typeof supportsRoleAssets, 'function')
    assert.equal(supportsRoleAssets?.('doubao-seedance-2-0-260128'), true)
    assert.equal(supportsRoleAssets?.('doubao-seedance-2-0-fast-260528'), false)
    assert.equal(supportsRoleAssets?.('doubao-seedance-2-0-mini-260615'), false)
    assert.equal(supportsRoleAssets?.('doubao-seedance-2-5-260628'), false)
  })

  test('rejects non-http media and Seedance media count overflow', () => {
    const media: VideoMedia = {
      images: Array.from(
        { length: 10 },
        (_, index) => `https://example.com/${index}.jpg`
      ),
      videos: [],
      audios: [],
    }

    assert.match(
      validateMediaLimits('doubao-seedance-2-0-260128', media) || '',
      /9/
    )
    assert.match(
      validateMediaLimits('doubao-seedance-2-0-260128', {
        images: ['data:image/png;base64,abc'],
        videos: [],
        audios: [],
      }) || '',
      /HTTP/
    )
  })

  test('honors API key model limits when they are enabled', () => {
    assert.deepEqual(
      filterModelsForApiKey(['doubao-seedance-2-0-260128', 'other-model'], {
        model_limits_enabled: true,
        model_limits: 'other-model',
      }),
      ['other-model']
    )
    assert.deepEqual(
      filterModelsForApiKey(['doubao-seedance-2-0-260128', 'other-model'], {
        model_limits_enabled: false,
        model_limits: '',
      }),
      ['doubao-seedance-2-0-260128', 'other-model']
    )
  })

  test('builds a runnable cURL command for the current application origin', () => {
    const command = buildVideoTaskCurl(
      {
        model: 'doubao-seedance-2-0-260128',
        content: [{ type: 'text', text: 'test prompt' }],
        ratio: '16:9',
        duration: 11,
        generate_audio: true,
        watermark: false,
      },
      'http://localhost:3003'
    )

    assert.match(
      command,
      /curl -X POST 'http:\/\/localhost:3003\/api\/v3\/contents\/generations\/tasks'/
    )
  })

  test('allows an optional task expiry to be cleared', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      model: 'doubao-seedance-2-0-260128',
      prompt: 'test prompt',
      imageSource: 'url',
      assetIds: [],
      media: { images: [], videos: [], audios: [] },
      resolution: '',
      ratio: '16:9',
      duration: 11,
      executionExpiresAfter: Number.NaN,
      generateAudio: true,
      watermark: false,
      returnLastFrame: false,
    })

    assert.equal(result.success, true)
  })

  test('allows Seedance 2.5 expanded reference media limits', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      ...DEFAULT_VIDEO_FORM,
      model: 'doubao-seedance-2-5-260628',
      media: {
        images: Array.from(
          { length: 30 },
          (_, index) => `https://example.com/image-${index}.png`
        ),
        videos: Array.from(
          { length: 10 },
          (_, index) => `https://example.com/video-${index}.mp4`
        ),
        audios: Array.from(
          { length: 10 },
          (_, index) => `https://example.com/audio-${index}.mp3`
        ),
      },
    })

    assert.equal(result.success, true)
  })

  for (const [kind, count, extension] of [
    ['images', 31, 'png'],
    ['videos', 11, 'mp4'],
    ['audios', 11, 'mp3'],
  ] as const) {
    test(`rejects Seedance 2.5 ${kind} above its limit`, () => {
      const result = createVideoGenerationSchema(t).safeParse({
        ...DEFAULT_VIDEO_FORM,
        model: 'doubao-seedance-2-5-260628',
        media: {
          images: [],
          videos: [],
          audios: [],
          [kind]: Array.from(
            { length: count },
            (_, index) => `https://example.com/${kind}-${index}.${extension}`
          ),
        },
      })

      assert.equal(result.success, false)
    })
  }

  test('rejects role assets for Seedance 2.5', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      ...DEFAULT_VIDEO_FORM,
      model: 'doubao-seedance-2-5-260628',
      imageSource: 'asset',
      assetIds: ['asset-20260401123823-6d4x2'],
      media: { images: [], videos: [], audios: [] },
    })

    assert.equal(result.success, false)
  })

  test('accepts a role asset as the only generation input', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      ...DEFAULT_VIDEO_FORM,
      prompt: '',
      imageSource: 'asset',
      assetIds: ['asset-20260401123823-6d4x2'],
      media: { images: [], videos: [], audios: [] },
    })

    assert.equal(result.success, true)
  })

  test('rejects more than nine role assets', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      ...DEFAULT_VIDEO_FORM,
      imageSource: 'asset',
      assetIds: Array.from(
        { length: 10 },
        (_, index) => `asset-2026040112382${index}-6d4x2`
      ),
      media: { images: [], videos: [], audios: [] },
    })

    assert.equal(result.success, false)
  })

  test('rejects malformed project asset IDs', () => {
    const result = createVideoGenerationSchema(t).safeParse({
      ...DEFAULT_VIDEO_FORM,
      imageSource: 'asset',
      assetIds: ['asset-invalid'],
      media: { images: [], videos: [], audios: [] },
    })

    assert.equal(result.success, false)
  })
})
