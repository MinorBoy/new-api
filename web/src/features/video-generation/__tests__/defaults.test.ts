import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { DEFAULT_VIDEO_FORM } from '../lib/defaults'
import { buildVideoRequest } from '../lib/request'

describe('Seedance video defaults', () => {
  test('starts with the supplied Seedance 2.0 multimodal sample', () => {
    assert.equal(DEFAULT_VIDEO_FORM.model, 'doubao-seedance-2-0-260128')
    assert.equal(DEFAULT_VIDEO_FORM.media.images.length, 2)
    assert.equal(DEFAULT_VIDEO_FORM.media.videos.length, 1)
    assert.equal(DEFAULT_VIDEO_FORM.media.audios.length, 1)
    assert.equal(DEFAULT_VIDEO_FORM.ratio, '16:9')
    assert.equal(DEFAULT_VIDEO_FORM.duration, 11)
    assert.equal(DEFAULT_VIDEO_FORM.generateAudio, true)
    assert.equal(DEFAULT_VIDEO_FORM.watermark, false)
  })

  test('serializes exactly the supplied default request fields', () => {
    const request = buildVideoRequest(DEFAULT_VIDEO_FORM)

    assert.deepEqual(Object.keys(request).sort(), [
      'content',
      'duration',
      'generate_audio',
      'model',
      'ratio',
      'watermark',
    ])
  })
})
