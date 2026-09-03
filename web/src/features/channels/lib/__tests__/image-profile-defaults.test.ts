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
*/
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
