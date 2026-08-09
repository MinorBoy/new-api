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

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import type { QueryClient as QueryClientType } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.defineProperty(browserWindow.document, 'compatMode', {
  value: 'CSS1Compat',
})
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  customElements: browserWindow.customElements,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  matchMedia: browserWindow.matchMedia.bind(browserWindow),
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
for (const [key, value] of Object.entries(browserGlobals)) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    writable: true,
    value,
  })
}

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { I18nextProvider } = await import('react-i18next')
const { api } = await import('@/lib/api')
const originalApiAdapter = api.defaults.adapter
const { GroupRoutingTargetsDialog } =
  await import('../group-routing-targets-dialog')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

type RequestBody = {
  model?: string
  page: number
  page_size: number
}

const requests: RequestBody[] = []
let cleanupFailure = false
const targetItems = [
  {
    model: 'seedance-2.0',
    channel_id: 23,
    channel_name: 'Matched channel',
    target_name: 'matched-target',
    upstream_model: 'vendor-matched',
    cost_variant_key: 'default',
    target_priority: 100,
    supports_real_person: true,
    cost_mode: 'per_duration',
    cost_rule_id: 101,
    cost_rule_version: 1,
    target_key: 'grt_match',
    status: 'matched',
    issues: [],
  },
  {
    model: 'seedance-2.0',
    channel_id: 24,
    channel_name: 'Mismatched channel',
    target_name: 'mismatched-target',
    upstream_model: 'vendor-mismatched',
    cost_variant_key: 'default',
    target_priority: 50,
    supports_real_person: false,
    cost_mode: 'per_request',
    cost_rule_id: 102,
    cost_rule_version: 1,
    target_key: 'grt_mismatch',
    status: 'real_person_mismatch',
    issues: ['real_person_mismatch'],
  },
] as const

function responseData(items = targetItems, staleExclusions = 2) {
  return {
    success: true,
    data: {
      items,
      summary: {
        models: 1,
        matched_models: 1,
        targets: 2,
        matched_targets: 1,
        stale_exclusions: staleExclusions,
      },
      facets: {
        models: ['seedance-2.0'],
        channels: [
          { id: 23, name: 'Matched channel' },
          { id: 24, name: 'Mismatched channel' },
        ],
        cost_modes: ['per_duration', 'per_request'],
        statuses: ['matched', 'real_person_mismatch'],
      },
      page: 1,
      page_size: 25,
      total: items.length,
    },
  }
}

api.defaults.adapter = async (config) => {
  const body = JSON.parse(String(config.data ?? '{}')) as RequestBody
  requests.push(body)
  if (cleanupFailure && body.page_size === 100) {
    throw new Error('cleanup failed')
  }
  return {
    data: responseData(),
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

after(() => {
  api.defaults.adapter = originalApiAdapter
  for (const key of Object.keys(browserGlobals)) {
    const previousDescriptor = previousBrowserGlobals[key]
    if (previousDescriptor === undefined) {
      delete (globalThis as Record<string, unknown>)[key]
      continue
    }
    Object.defineProperty(globalThis, key, previousDescriptor)
  }
  browserWindow.close()
})

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  requests.length = 0
  cleanupFailure = false
})

function findButton(name: string): HTMLButtonElement {
  const button = [...browserWindow.document.querySelectorAll('button')].find(
    (candidate) =>
      candidate.getAttribute('aria-label') === name ||
      candidate.textContent?.trim() === name
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

async function waitForText(text: string) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    if ((browserWindow.document.body.textContent ?? '').includes(text)) return
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail(`Timed out waiting for ${text}`)
}

async function waitForRequest(predicate: (request: RequestBody) => boolean) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    if (requests.some(predicate)) return
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail('Timed out waiting for preview request')
}

async function mountDialog(excludeMatched = false): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClientType
  getSource: () => string
}> {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  let source = JSON.stringify({
    premium: {
      status: 'draft',
      routing_source: 'default',
      excluded_target_keys: excludeMatched
        ? ['grt_match', 'grt_stale_a', 'grt_stale_b']
        : ['grt_stale_a', 'grt_stale_b'],
    },
  })

  function Fixture() {
    const [value, setValue] = useState(source)
    source = value
    return (
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <GroupRoutingTargetsDialog
            open
            groupName='premium'
            groupRoutingRequirements={value}
            onOpenChange={() => {}}
            onChange={setValue}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
  }

  await act(async () => root.render(<Fixture />))
  await act(async () => undefined)
  return { root, container, queryClient, getSource: () => source }
}

async function unmountDialog(mounted: Awaited<ReturnType<typeof mountDialog>>) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

test('excludes matched targets, restores excluded targets, and never force-includes mismatches', async () => {
  const mounted = await mountDialog()
  try {
    await waitForText('Matched channel')
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /Matched channel/
    )
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /Mismatched channel/
    )
    assert.ok(
      browserWindow.document.querySelectorAll(
        'button[aria-label="Exclude target"]'
      ).length >= 1
    )
    assert.equal(
      browserWindow.document.querySelectorAll(
        'button[aria-label="Force include"]'
      ).length,
      0
    )

    await act(async () => findButton('Exclude target').click())
    await act(async () => findButton('Restore target').click())
    assert.equal(
      JSON.parse(mounted.getSource()).premium.excluded_target_keys.includes(
        'grt_match'
      ),
      false
    )
  } finally {
    await unmountDialog(mounted)
  }
})

test('cleans stale exclusions using the complete unfiltered target set', async () => {
  const mounted = await mountDialog(true)
  try {
    await waitForText('stale exclusions')
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /2 stale exclusions/
    )
    await act(async () => findButton('Clean stale exclusions').click())
    await act(async () => undefined)

    assert.deepEqual(
      JSON.parse(mounted.getSource()).premium.excluded_target_keys,
      ['grt_match']
    )
    assert.ok(
      requests.some(
        (request) =>
          request.page === 1 &&
          request.page_size === 100 &&
          request.model === undefined
      )
    )
  } finally {
    await unmountDialog(mounted)
  }
})

test('sends model filters to the preview endpoint', async () => {
  const mounted = await mountDialog()
  try {
    const modelInput = browserWindow.document.querySelector(
      'input[aria-label="Filter by model"]'
    )
    assert.ok(modelInput instanceof browserWindow.HTMLInputElement)
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        browserWindow.HTMLInputElement.prototype,
        'value'
      )?.set
      assert.ok(setter)
      setter.call(modelInput, 'seedance-pro')
      modelInput.dispatchEvent(
        new browserWindow.Event('input', { bubbles: true })
      )
      modelInput.dispatchEvent(
        new browserWindow.Event('change', { bubbles: true })
      )
    })
    await waitForRequest((request) => request.model === 'seedance-pro')
  } finally {
    await unmountDialog(mounted)
  }
})

test('shows a recoverable error when stale exclusion cleanup fails', async () => {
  const mounted = await mountDialog(true)
  try {
    await waitForText('stale exclusions')
    cleanupFailure = true
    await act(async () => findButton('Clean stale exclusions').click())
    await waitForText('Unable to clean stale exclusions')
  } finally {
    await unmountDialog(mounted)
  }
})
