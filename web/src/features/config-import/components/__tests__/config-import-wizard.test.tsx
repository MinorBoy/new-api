/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import {
  Window,
  type HTMLButtonElement,
  type HTMLElement as HappyHTMLElement,
  type HTMLInputElement,
} from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { ConfigImportWizard } from '../../index'
import type {
  ConfigImportBatchDetail,
  ConfigImportPricingReviewRequest,
  ConfigImportRouteReviewsRequest,
} from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  localStorage: browserWindow.localStorage,
  matchMedia: browserWindow.matchMedia.bind(browserWindow),
  HTMLSelectElement: browserWindow.HTMLSelectElement,
  sessionStorage: browserWindow.sessionStorage,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

function batch(
  status: ConfigImportBatchDetail['status'],
  allowedActions: string[],
  overrides: Partial<ConfigImportBatchDetail> = {}
): ConfigImportBatchDetail {
  return {
    id: 12,
    schema_version: 1,
    template_version: 'v1',
    source_sha256: 'source',
    payload_sha256: 'payload',
    status,
    created_by: 1,
    item_counts: {
      channels: 0,
      channel_lines: 0,
      model_skus: 0,
      sale_proposals: 0,
      cost_rule_drafts: 0,
      model_mappings: 0,
      route_blueprints: 0,
      sources: 0,
      unresolved_variants: 0,
    },
    issue_count: 0,
    allowed_actions: allowedActions,
    activated_at: status === 'published' ? 123 : null,
    created_at: 1,
    updated_at: 1,
    items: [],
    issues: [],
    activation_preview: null,
    ...overrides,
  }
}

function publishedForActivation(
  overrides: Partial<ConfigImportBatchDetail> = {}
): ConfigImportBatchDetail {
  return batch('published', ['activate'], {
    activated_at: null,
    activation_preview: {
      ready: true,
      channel_count: 2,
      policy_count: 3,
      target_count: 13,
      retire_target_count: 67,
      blockers: [],
    },
    ...overrides,
  })
}

function bindingBatch(): ConfigImportBatchDetail {
  const currentBatch = batch('binding', ['bind'])
  currentBatch.items = [
    {
      id: 1,
      entity_type: 'channel_lines',
      business_id: 'line-1',
      entity_hash: 'line-1-hash',
      canonical_json: '{"line_ref":"line-1","display_name":"Imported line"}',
      state: 'new',
      source_ref: '渠道!4',
      source_sheet: '渠道',
      source_row: 4,
    },
  ]
  return currentBatch
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount(
  options: {
    currentBatch?: ConfigImportBatchDetail
    restoreBatchID?: number
    onLoadBatch?: (id: number) => Promise<ConfigImportBatchDetail>
    onStage?: (id: number) => Promise<ConfigImportBatchDetail>
    onValidate?: (id: number) => Promise<ConfigImportBatchDetail>
    onPublish?: (id: number) => Promise<ConfigImportBatchDetail>
    onCopyForBinding?: (id: number) => Promise<ConfigImportBatchDetail>
    onActivate?: (id: number) => Promise<ConfigImportBatchDetail>
    onRefreshCache?: (id: number) => Promise<ConfigImportBatchDetail>
    onSavePricingReview?: (
      id: number,
      request: ConfigImportPricingReviewRequest
    ) => Promise<ConfigImportBatchDetail>
    onSaveRouteReviews?: (
      id: number,
      request: ConfigImportRouteReviewsRequest
    ) => Promise<ConfigImportBatchDetail>
    onLoadChannels?: () => Promise<
      Array<{ id: number; name: string; status: number }>
    >
  } = {}
) {
  const calls: string[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ConfigImportWizard
          batch={options.currentBatch}
          restoreBatchID={options.restoreBatchID}
          onLoadBatch={async (id) => {
            calls.push(`load:${id}`)
            return options.onLoadBatch?.(id) ?? batch('staged', ['validate'])
          }}
          onStage={async (id) => {
            calls.push(`stage:${id}`)
            return options.onStage?.(id) ?? batch('staged', ['validate'])
          }}
          onValidate={async (id) => {
            calls.push(`validate:${id}`)
            return options.onValidate?.(id) ?? batch('ready', ['publish'])
          }}
          onPublish={async (id) => {
            calls.push(`publish:${id}`)
            return options.onPublish?.(id) ?? publishedForActivation()
          }}
          onCopyForBinding={async (id) => {
            calls.push(`copy:${id}`)
            return (
              options.onCopyForBinding?.(id) ??
              batch('binding', ['bind'], {
                id: id + 1,
                copied_from_batch_id: id,
              })
            )
          }}
          onActivate={async (id) => {
            calls.push(`activate:${id}`)
            return options.onActivate?.(id) ?? batch('published', [])
          }}
          onRefreshCache={async (id) => {
            calls.push(`refresh:${id}`)
            return options.onRefreshCache?.(id) ?? batch('published', [])
          }}
          onSavePricingReview={async (id, request) => {
            calls.push(`pricing:${id}`)
            return (
              options.onSavePricingReview?.(id, request) ??
              batch('staged', ['stage'])
            )
          }}
          onSaveRouteReviews={async (id, request) => {
            calls.push(`routes:${id}`)
            return (
              options.onSaveRouteReviews?.(id, request) ??
              batch('staged', ['validate'])
            )
          }}
          onLoadChannels={options.onLoadChannels}
        />
      </I18nextProvider>
    )
  })
  return { container, root, calls }
}

test('loads existing channels as binding candidates', async () => {
  const mounted = await mount({
    currentBatch: bindingBatch(),
    onLoadChannels: async () => [
      { id: 25, name: 'Disabled mock channel', status: 0 },
    ],
  })
  try {
    await act(async () => undefined)
    const select = mounted.container.querySelector('select')
    assert.ok(select instanceof browserWindow.HTMLSelectElement)
    assert.equal(
      [...select.options].some((option) => option.value === '25'),
      true
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('renders a scrollable container for a binding batch', async () => {
  const mounted = await mount({ currentBatch: bindingBatch() })
  try {
    const wizard = mounted.container.querySelector(
      '[aria-labelledby="config-import-wizard-title"]'
    )
    assert.ok(wizard instanceof browserWindow.HTMLElement)
    assert.equal(wizard.classList.contains('min-h-0'), true)
    assert.equal(wizard.classList.contains('flex-1'), true)
    assert.equal(wizard.classList.contains('overflow-auto'), true)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

function button(container: HappyHTMLElement, text: string): HTMLButtonElement {
  const value = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === text
  ) as HTMLButtonElement | undefined
  assert.ok(value)
  return value
}

test('restores a staged batch and uses server responses for stage and validate transitions', async () => {
  const mounted = await mount({
    currentBatch: batch('binding', ['stage']),
    onStage: async () => batch('staged', ['validate']),
    onValidate: async () => batch('ready', ['publish']),
  })
  try {
    assert.match(mounted.container.textContent ?? '', /Channel bindings/)
    await act(async () => button(mounted.container, 'Stage import').click())
    assert.match(mounted.container.textContent ?? '', /Conflict resolution/)
    await act(async () => button(mounted.container, 'Validate import').click())
    assert.match(mounted.container.textContent ?? '', /Publish review/)
    assert.deepEqual(mounted.calls, ['stage:12', 'validate:12'])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('restores a staged batch from the recovery identifier', async () => {
  const mounted = await mount({ restoreBatchID: 44 })
  try {
    assert.match(mounted.container.textContent ?? '', /Conflict resolution/)
    assert.deepEqual(mounted.calls, ['load:44'])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('returns to routing diffs when publish reports a stale baseline', async () => {
  const staleError = Object.assign(new Error('stale'), {
    code: 'STALE_BASE_VERSION',
  })
  const mounted = await mount({
    currentBatch: batch('ready', ['publish']),
    onPublish: async () => {
      throw staleError
    },
  })
  try {
    const confirm = mounted.container.querySelector(
      '[aria-label="Confirm publish"]'
    ) as HTMLInputElement | null
    assert.ok(confirm)
    await act(async () => confirm.click())
    await act(async () => button(mounted.container, 'Publish import').click())
    assert.ok(
      mounted.container.querySelector('#config-import-routing-title')
    )
    assert.equal(
      mounted.container.querySelector('#config-import-publish-review-title'),
      null
    )
    assert.match(mounted.container.textContent ?? '', /stale/i)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('shows the publish result after a successful publish', async () => {
  const mounted = await mount({
    currentBatch: batch('staged', ['validate']),
    onStage: async () => batch('staged', ['validate']),
    onValidate: async () => batch('ready', ['publish']),
    onPublish: async () => publishedForActivation(),
    onActivate: async () => batch('published', []),
  })
  try {
    await act(async () => button(mounted.container, 'Continue').click())
    assert.match(mounted.container.textContent ?? '', /Pricing review/)
    await act(async () => button(mounted.container, 'Continue').click())
    assert.match(mounted.container.textContent ?? '', /Routing diff/)
    await act(async () => button(mounted.container, 'Continue').click())
    assert.match(mounted.container.textContent ?? '', /Publish review/)
    const confirm = mounted.container.querySelector(
      '[aria-label="Confirm publish"]'
    ) as HTMLInputElement | null
    assert.ok(confirm)
    await act(async () => confirm.click())
    await act(async () => button(mounted.container, 'Publish import').click())

    assert.match(mounted.container.textContent ?? '', /Route activation/)
    const activationConfirm = mounted.container.querySelector(
      '[aria-label="Confirm activation"]'
    ) as HTMLInputElement | null
    assert.ok(activationConfirm)
    await act(async () => activationConfirm.click())
    await act(async () => button(mounted.container, 'Activate import').click())

    assert.match(mounted.container.textContent ?? '', /Published/)
    assert.match(
      mounted.container.textContent ?? '',
      /The published configuration is active\./
    )
    assert.equal(
      [...mounted.container.querySelectorAll('button')].some(
        (candidate) => candidate.textContent === 'Publish import'
      ),
      false
    )
    assert.deepEqual(mounted.calls, [
      'pricing:12',
      'stage:12',
      'routes:12',
      'validate:12',
      'publish:12',
      'activate:12',
    ])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('does not activate a published batch with blockers', async () => {
  const mounted = await mount({
    currentBatch: publishedForActivation({
      allowed_actions: ['activate', 'copy_for_binding'],
      activation_preview: {
        ready: false,
        channel_count: 1,
        policy_count: 1,
        target_count: 1,
        retire_target_count: 0,
        blockers: [
          {
            code: 'ACTIVATION_CHANNEL_KEY_MISSING',
            message: 'Channel 9 does not have an API key.',
            channel_id: 9,
          },
        ],
      },
    }),
  })
  try {
    assert.match(
      mounted.container.textContent ?? '',
      /ACTIVATION_CHANNEL_KEY_MISSING/
    )
    assert.equal(button(mounted.container, 'Activate import').disabled, true)
    await act(async () =>
      button(mounted.container, 'Copy as new binding batch').click()
    )
    assert.match(mounted.container.textContent ?? '', /Channel bindings/)
    assert.equal(
      mounted.calls.some((call) => call.startsWith('activate:')),
      false
    )
    assert.equal(mounted.calls.filter((call) => call === 'copy:12').length, 1)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('returns to routing diffs when publish finds an unbound route line', async () => {
  const publishError = Object.assign(
    new Error(
      'route target "route-target/MAP-MEGABYAI-R122-480" references unbound line "megabyai-fast-real-person"'
    ),
    {
      response: {
        data: { code: 'PUBLISH_LINE_UNBOUND' },
      },
    }
  )
  const mounted = await mount({
    currentBatch: batch('ready', ['publish']),
    onPublish: async () => {
      throw publishError
    },
  })
  try {
    const confirmation = mounted.container.querySelector(
      '[aria-label="Confirm publish"]'
    ) as HTMLInputElement | null
    assert.ok(confirmation)
    await act(async () => confirmation.click())
    await act(async () => button(mounted.container, 'Publish import').click())

    assert.ok(
      mounted.container.querySelector('#config-import-routing-title')
    )
    assert.equal(
      mounted.container.querySelector('#config-import-publish-review-title'),
      null
    )
    assert.match(mounted.container.textContent ?? '', /references unbound line/)
    assert.deepEqual(mounted.calls, ['publish:12'])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('recovers an activated batch after cache refresh failure', async () => {
  const cacheError = Object.assign(new Error('cache refresh pending'), {
    code: 'ACTIVATION_CACHE_REFRESH_PENDING',
  })
  const pendingBatch = batch('published', ['refresh_cache'], {
    activated_at: 123,
    issues: [
      {
        id: 9,
        severity: 'warning',
        code: 'ACTIVATION_CACHE_REFRESH_PENDING',
        message: 'Activated configuration cache refresh is pending.',
        resolution_status: 'open',
      },
    ],
  })
  const mounted = await mount({
    currentBatch: publishedForActivation(),
    onActivate: async () => {
      throw cacheError
    },
    onLoadBatch: async () => pendingBatch,
    onRefreshCache: async () => batch('published', []),
  })
  try {
    const confirmation = mounted.container.querySelector(
      '[aria-label="Confirm activation"]'
    ) as HTMLInputElement | null
    assert.ok(confirmation)
    await act(async () => confirmation.click())
    await act(async () => button(mounted.container, 'Activate import').click())

    assert.match(mounted.container.textContent ?? '', /Published/)
    assert.match(
      mounted.container.textContent ?? '',
      /cache refresh is still pending/i
    )
    assert.deepEqual(mounted.calls, ['activate:12', 'load:12'])
    assert.equal(
      [...mounted.container.querySelectorAll('button')].some(
        (candidate) => candidate.textContent === 'Activate import'
      ),
      false
    )

    await act(async () =>
      button(mounted.container, 'Retry cache refresh').click()
    )
    assert.deepEqual(mounted.calls, ['activate:12', 'load:12', 'refresh:12'])
    assert.doesNotMatch(
      mounted.container.textContent ?? '',
      /configuration changed, but cache refresh is still pending/i
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('keeps the legacy publish cache issue code in the recovery result', async () => {
  const mounted = await mount({
    currentBatch: batch('published', ['refresh_cache'], {
      issues: [
        {
          id: 10,
          severity: 'warning',
          code: 'CACHE_REFRESH_PENDING',
          message: 'Published configuration cache refresh is pending.',
          resolution_status: 'open',
        },
      ],
    }),
  })
  try {
    const content = mounted.container.textContent ?? ''
    assert.match(content, /CACHE_REFRESH_PENDING/)
    assert.doesNotMatch(content, /ACTIVATION_CACHE_REFRESH_PENDING/)
    assert.equal(
      button(mounted.container, 'Retry cache refresh').disabled,
      false
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('shows retry guidance for publish failures and a terminal published summary', async () => {
  const failed = await mount({
    currentBatch: batch('publish_failed', ['validate']),
  })
  try {
    assert.match(failed.container.textContent ?? '', /retry/i)
    await act(async () => button(failed.container, 'Retry validation').click())
    assert.deepEqual(failed.calls, ['validate:12'])
  } finally {
    await act(async () => failed.root.unmount())
  }

  const published = await mount({ currentBatch: batch('published', []) })
  try {
    assert.match(published.container.textContent ?? '', /Published/)
    assert.match(published.container.textContent ?? '', /created/i)
    assert.equal(
      [...published.container.querySelectorAll('button')].some(
        (candidate) => candidate.textContent === 'Publish import'
      ),
      false
    )
  } finally {
    await act(async () => published.root.unmount())
  }
})

test('copies a published batch into a fresh binding batch', async () => {
  const mounted = await mount({
    currentBatch: batch('published', ['copy_for_binding'], {
      activated_at: 123,
    }),
    onCopyForBinding: async () =>
      batch('binding', ['bind'], {
        id: 13,
        copied_from_batch_id: 12,
        activated_at: null,
      }),
  })
  try {
    await act(async () =>
      button(mounted.container, 'Copy as new binding batch').click()
    )
    assert.match(mounted.container.textContent ?? '', /Channel bindings/)
    assert.deepEqual(mounted.calls, ['copy:12'])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('does not show the copy action when the backend disallows it', async () => {
  const mounted = await mount({
    currentBatch: batch('published', [], { activated_at: 123 }),
  })
  try {
    assert.equal(
      [...mounted.container.querySelectorAll('button')].some(
        (candidate) => candidate.textContent === 'Copy as new binding batch'
      ),
      false
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
