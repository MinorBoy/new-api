import assert from 'node:assert/strict'
import test from 'node:test'

import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import { PricingToolbar } from './pricing-toolbar'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

test('view mode buttons have individual accessible names', () => {
  const html = renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(PricingToolbar, {
        filteredCount: 1,
        totalCount: 1,
        sortBy: 'name',
        onSortChange: () => {},
        tokenUnit: 'M',
        onTokenUnitChange: () => {},
        showRechargePrice: false,
        onRechargePriceChange: () => {},
        viewMode: 'card',
        onViewModeChange: () => {},
        quotaTypeFilter: 'all',
        endpointTypeFilter: 'all',
        vendorFilter: 'all',
        groupFilter: 'all',
        tagFilter: 'all',
        onQuotaTypeChange: () => {},
        onEndpointTypeChange: () => {},
        onVendorChange: () => {},
        onGroupChange: () => {},
        onTagChange: () => {},
        vendors: [],
        groups: [],
        tags: [],
        models: [],
        hasActiveFilters: false,
        activeFilterCount: 0,
        onClearFilters: () => {},
      })
    )
  )

  assert.match(html, /<button[^>]*aria-label="Card view"/)
  assert.match(html, /<button[^>]*aria-label="Table view"/)
})
