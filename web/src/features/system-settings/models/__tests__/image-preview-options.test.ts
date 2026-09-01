import assert from 'node:assert/strict'
import test from 'node:test'

import {
  getImagePreviewEndpointOptions,
  getImagePreviewOptions,
  normalizeImagePreviewSelection,
} from '../image-preview-options'
import { parseImageRoutingPolicy } from '../image-routing-policy'

const catalog = JSON.stringify({
  version: 1,
  models: {
    'gpt-image-1': {
      endpoints: {
        generations: {
          capability: {
            enabled: true,
            sizes: ['1024x1024', '1536x1024'],
            qualities: ['medium'],
            response_formats: ['b64_json'],
          },
        },
        edits: { capability: { enabled: false } },
      },
    },
  },
})

test('preview options follow the configured catalog and routing groups', () => {
  assert.deepEqual(getImagePreviewOptions(catalog, '{"groups":{"image":{}}}'), {
    groups: ['default', 'image'],
    models: ['gpt-image-1'],
    endpoints: ['generations'],
    sizes: ['1024x1024', '1536x1024'],
    qualities: ['medium'],
    responseFormats: ['b64_json'],
  })
})

test('preview selection resets unsupported values after model or endpoint changes', () => {
  const selection = normalizeImagePreviewSelection(
    {
      group: 'missing',
      model: 'missing',
      endpoint: 'edits',
      size: 'missing',
      quality: 'missing',
      response_format: 'missing',
      n: 0,
    },
    catalog,
    '{"groups":{"image":{}}}'
  )

  assert.deepEqual(selection, {
    group: 'default',
    model: 'gpt-image-1',
    endpoint: 'generations',
    size: '1024x1024',
    quality: 'medium',
    response_format: 'b64_json',
    n: 1,
  })
})

test('endpoint options come from the selected model and endpoint', () => {
  const options = getImagePreviewEndpointOptions(
    catalog,
    'gpt-image-1',
    'generations'
  )
  assert.deepEqual(options, {
    endpoints: ['generations'],
    sizes: ['1024x1024', '1536x1024'],
    qualities: ['medium'],
    responseFormats: ['b64_json'],
  })
})

test('image routing policy accepts cost weighted tolerance', () => {
  assert.deepEqual(
    parseImageRoutingPolicy(
      JSON.stringify({
        version: 1,
        default: { strategy: 'cost_weighted', cost_tolerance_bps: 1000 },
      })
    ),
    {
      version: 1,
      default: { strategy: 'cost_weighted', cost_tolerance_bps: 1000 },
    }
  )
})

test('image routing policy rejects invalid tolerance combinations', () => {
  assert.throws(() =>
    parseImageRoutingPolicy(
      JSON.stringify({
        version: 1,
        default: { strategy: 'manual', cost_tolerance_bps: 1000 },
      })
    )
  )
  assert.throws(() =>
    parseImageRoutingPolicy(
      JSON.stringify({
        version: 1,
        default: { strategy: 'cost_weighted', cost_tolerance_bps: 10001 },
      })
    )
  )
  assert.throws(() =>
    parseImageRoutingPolicy(
      JSON.stringify({
        version: 2,
        default: { strategy: 'cost_weighted', cost_tolerance_bps: 1000 },
      })
    )
  )
})
