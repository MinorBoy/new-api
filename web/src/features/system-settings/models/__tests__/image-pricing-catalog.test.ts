import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  calculateExpectedMarginBps,
  flattenImagePricingCatalog,
  updateCatalogSalePrices,
  validateSalePrice,
} from '../image-pricing-catalog'

const catalogFixture = {
  version: 1,
  models: {
    'gpt-image-2': {
      profile: 'openai_images',
      profile_version: 1,
      endpoints: {
        generations: {
          capability: {
            enabled: true,
            sizes: ['4096x4096', '1024x1024'],
            qualities: ['medium'],
            response_formats: ['b64_json'],
            max_n: 4,
          },
          default_size: '1024x1024',
          default_quality: 'medium',
          default_response_format: 'b64_json',
        },
      },
      skus: {
        'gen-4096x4096-medium': {
          endpoint: 'generations',
          size: '4096x4096',
          quality: 'medium',
          unit: 'image',
          sale_price_usd: '0.08',
        },
        'gen-1024x1024-medium': {
          endpoint: 'generations',
          size: '1024x1024',
          quality: 'medium',
          unit: 'image',
          sale_price_usd: '0.03',
        },
      },
    },
  },
} as const

describe('image pricing catalog helpers', () => {
  test('flattens legacy unified SKUs into the tier grid', () => {
    const rows = flattenImagePricingCatalog(catalogFixture)
    assert.deepEqual(
      rows.map((row) => row.id),
      [
        'gpt-image-2|gen-1k-high',
        'gpt-image-2|gen-1k-low',
        'gpt-image-2|gen-1k-medium',
        'gpt-image-2|gen-2k-high',
        'gpt-image-2|gen-2k-low',
        'gpt-image-2|gen-2k-medium',
        'gpt-image-2|gen-4k-high',
        'gpt-image-2|gen-4k-low',
        'gpt-image-2|gen-4k-medium',
      ]
    )
  })

  test('shows the tier grid for a legacy unified image catalog', () => {
    const rows = flattenImagePricingCatalog(catalogFixture)
    assert.equal(rows.length, 9)
    assert.equal(
      rows.find((row) => row.skuKey === 'gen-1k-medium')?.configured,
      true
    )
    assert.equal(
      rows.find((row) => row.skuKey === 'gen-4k-medium')?.configured,
      true
    )
    assert.equal(
      rows.find((row) => row.skuKey === 'gen-4k-medium')?.salePriceUSD,
      '0.08'
    )
  })

  test('updates only sale prices and preserves catalog capabilities', () => {
    const updated = updateCatalogSalePrices(catalogFixture, {
      'gpt-image-2|gen-1024x1024-medium': '0.035',
    })
    const updatedSKU =
      updated.models['gpt-image-2']?.skus?.['gen-1024x1024-medium']
    assert.equal(updatedSKU?.sale_price_usd, '0.035')
    assert.deepEqual(
      updated.models['gpt-image-2'].endpoints,
      catalogFixture.models['gpt-image-2'].endpoints
    )
  })

  test('rejects invalid sale prices and calculates safe margins', () => {
    assert.equal(validateSalePrice(''), 'Price is required')
    assert.equal(validateSalePrice('-0.01'), 'Price must be non-negative')
    assert.equal(
      validateSalePrice('0.1234567891'),
      'Price has too many decimal places'
    )
    assert.equal(calculateExpectedMarginBps('0.03', '0.02'), 3333)
    assert.equal(calculateExpectedMarginBps('0', '0.02'), null)
  })

  test('shows the complete resolution and quality grid for tiered catalogs', () => {
    const tiered = {
      version: 1,
      models: {
        image: {
          endpoints: {
            generations: {
              capability: { enabled: true, qualities: ['medium'] },
            },
          },
          skus: {
            'gen-1k-medium': {
              endpoint: 'generations',
              tier: '1k',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.02',
            },
          },
        },
      },
    }
    const rows = flattenImagePricingCatalog(tiered)
    assert.equal(rows.length, 9)
    assert.equal(
      rows.find((row) => row.skuKey === 'gen-1k-medium')?.configured,
      true
    )
    assert.equal(
      rows.find((row) => row.skuKey === 'gen-4k-high')?.configured,
      false
    )
  })

  test('adds a missing tier SKU and advertises its capability', () => {
    const updated = updateCatalogSalePrices(
      {
        version: 1,
        models: {
          image: {
            endpoints: {
              generations: {
                capability: { enabled: true, qualities: ['medium'] },
              },
            },
            skus: {},
          },
        },
      },
      { 'image|gen-4k-high': '0.08' }
    )
    const model = updated.models.image
    assert.equal(model.skus?.['gen-4k-high']?.tier, '4k')
    assert.deepEqual(
      model.endpoints?.generations?.capability?.resolution_tiers,
      ['4k']
    )
    assert.deepEqual(model.endpoints?.generations?.capability?.qualities, [
      'medium',
      'high',
    ])
  })

  test('keeps legacy endpoints out of the tiered grid', () => {
    const mixed = {
      version: 1,
      models: {
        image: {
          endpoints: {
            generations: {
              capability: { enabled: true, qualities: ['medium'] },
            },
            edits: {
              capability: {
                enabled: true,
                sizes: ['1024x1024'],
                qualities: ['medium'],
              },
            },
          },
          skus: {
            'gen-1k-medium': {
              endpoint: 'generations',
              tier: '1k',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.02',
            },
            'edit-1024x1024-medium': {
              endpoint: 'edits',
              size: '1024x1024',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.03',
            },
          },
        },
      },
    }
    const rows = flattenImagePricingCatalog(mixed)
    assert.equal(rows.filter((row) => row.endpoint === 'generations').length, 9)
    assert.equal(rows.filter((row) => row.endpoint === 'edits').length, 1)
    assert.equal(
      rows.find((row) => row.skuKey === 'edit-1024x1024-medium')?.configured,
      true
    )
  })
})

describe('Image 2.5 pricing catalog', () => {
  test('expands each configured model into an independent nine-cell matrix', () => {
    const rows = flattenImagePricingCatalog({
      version: 1,
      models: Object.fromEntries(
        ['gpt-image-2', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst'].map(
          (model) => [
            model,
            {
              profile: 'openai_images',
              profile_version: 1,
              endpoints: {
                generations: {
                  capability: {
                    enabled: true,
                    resolution_tiers: ['1k', '2k', '4k'],
                    qualities: ['low', 'medium', 'high'],
                    response_formats: ['b64_json'],
                    max_n: 4,
                  },
                },
              },
              skus: {
                'gen-1k-medium': {
                  endpoint: 'generations',
                  tier: '1k',
                  quality: 'medium',
                  unit: 'image',
                  sale_price_usd: '0.03',
                },
              },
            },
          ]
        )
      ),
    })

    assert.equal(rows.length, 27)
    for (const model of [
      'gpt-image-2',
      'gpt-image-2.5-flare',
      'gpt-image-2.5-sunburst',
    ]) {
      const modelRows = rows.filter((row) => row.model === model)
      assert.equal(modelRows.length, 9)
      assert.ok(modelRows.some((row) => row.id === `${model}|gen-4k-high`))
    }
  })

  test('updates one Image 2.5 price without changing the other models', () => {
    const catalog = {
      version: 1,
      models: {
        'gpt-image-2': {
          skus: {
            'gen-1k-medium': {
              endpoint: 'generations',
              tier: '1k',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.03',
            },
          },
        },
        'gpt-image-2.5-flare': {
          skus: {
            'gen-1k-medium': {
              endpoint: 'generations',
              tier: '1k',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.04',
            },
          },
        },
        'gpt-image-2.5-sunburst': {
          skus: {
            'gen-1k-medium': {
              endpoint: 'generations',
              tier: '1k',
              quality: 'medium',
              unit: 'image',
              sale_price_usd: '0.05',
            },
          },
        },
      },
    }
    const updated = updateCatalogSalePrices(catalog, {
      'gpt-image-2.5-flare|gen-2k-high': '0.12',
    })

    assert.equal(
      updated.models['gpt-image-2.5-flare'].skus?.['gen-2k-high']
        ?.sale_price_usd,
      '0.12'
    )
    assert.equal(
      updated.models['gpt-image-2.5-sunburst'].skus?.['gen-1k-medium']
        ?.sale_price_usd,
      '0.05'
    )
    assert.equal(
      updated.models['gpt-image-2'].skus?.['gen-1k-medium']?.sale_price_usd,
      '0.03'
    )
  })
})
