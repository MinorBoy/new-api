import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { t } from 'i18next'

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
    assert.equal(request.watermark, false)
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

    assert.match(validateMediaLimits(media) || '', /9/)
    assert.match(
      validateMediaLimits({
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
})
