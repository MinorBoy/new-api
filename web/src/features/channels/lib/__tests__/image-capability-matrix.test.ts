import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  readImageCapabilityMatrix,
  writeImageCapabilityMatrix,
} from '../channel-form'

const allDisabled = {
  '1k:low': false,
  '1k:medium': false,
  '1k:high': false,
  '2k:low': false,
  '2k:medium': false,
  '2k:high': false,
  '4k:low': false,
  '4k:medium': false,
  '4k:high': false,
}

test('keeps Image 2.5 capability matrices independent, including an empty matrix', () => {
  let profile = writeImageCapabilityMatrix(
    undefined,
    'gpt-image-2.5-flare',
    allDisabled
  )
  profile = writeImageCapabilityMatrix(profile, 'gpt-image-2.5-sunburst', {
    ...allDisabled,
    '2k:high': true,
  })

  assert.deepEqual(
    readImageCapabilityMatrix(profile, 'gpt-image-2.5-flare'),
    allDisabled
  )
  assert.deepEqual(
    readImageCapabilityMatrix(profile, 'gpt-image-2.5-sunburst'),
    {
      ...allDisabled,
      '2k:high': true,
    }
  )
})
