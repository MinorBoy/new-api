import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DEFAULT_IMAGE_PROFILE_JSON,
  isEmptyImageProfile,
} from '../channel-form'

test('empty image profile values are only blank text or an empty object', () => {
  assert.equal(isEmptyImageProfile(undefined), true)
  assert.equal(isEmptyImageProfile('  '), true)
  assert.equal(isEmptyImageProfile('{}'), true)
  assert.equal(isEmptyImageProfile('{ }'), true)
  assert.equal(isEmptyImageProfile('{"profile":"custom"}'), false)
  assert.equal(isEmptyImageProfile('null'), false)
  assert.equal(isEmptyImageProfile('{invalid'), false)
})

test('default image profile contains standard OpenAI Images routes', () => {
  assert.deepEqual(JSON.parse(DEFAULT_IMAGE_PROFILE_JSON), {
    profile: 'openai_images',
    profile_version: 1,
    paths: {
      generations: '/v1/images/generations',
      edits: '/v1/images/edits',
    },
  })
})
