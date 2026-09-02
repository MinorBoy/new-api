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
import test, { after, before } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  RouterContextProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from '@tanstack/react-router'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { countUnsavedImagePricingChanges } from '../image-pricing-workbench-utils'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  customElements: browserWindow.customElements,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  HTMLTextAreaElement: browserWindow.HTMLTextAreaElement,
  SVGElement: browserWindow.SVGElement,
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

const catalog = JSON.stringify({
  version: 1,
  models: {
    'gpt-image-2': {
      profile: 'openai_images',
      endpoints: {
        generations: {
          capability: {
            enabled: true,
            sizes: ['1024x1024'],
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
})
const routing = JSON.stringify({
  version: 1,
  default: { strategy: 'lowest_cost' },
  groups: {},
})

for (const [key, value] of Object.entries(browserGlobals)) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    writable: true,
    value,
  })
}

const { createRoot } = await import('react-dom/client')
const { api } = await import('@/lib/api')
const { ImagePricingWorkbench } = await import('../image-pricing-workbench')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

let rejectSave = false
const savedRequests: Array<{ key: string; value: string }> = []
const originalApiAdapter = api.defaults.adapter
api.defaults.adapter = async (config) => {
  const url = config.url ?? ''
  if (url.includes('/api/cost-accounting/image-pricing')) {
    return {
      data: {
        success: true,
        message: '',
        data: {
          items: [
            {
              model: 'gpt-image-2',
              sku: 'gen-1024x1024-medium',
              known: true,
              minimum_cost_usd: '0.02',
              source_count: 2,
            },
          ],
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  if (url === '/api/option/' && config.method === 'put') {
    if (rejectSave) {
      throw new Error('permission denied')
    }
    const body = (
      typeof config.data === 'string' ? JSON.parse(config.data) : config.data
    ) as { key: string; value: string }
    savedRequests.push(body)
    return {
      data: { success: true, message: '' },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  if (url.includes('/api/routing-policies/image/preview')) {
    return {
      data: {
        success: true,
        message: '',
        data: { strategy: 'lowest_cost', sku: 'gen-1024x1024-medium' },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  throw new Error(`Unexpected test request: ${config.method} ${url}`)
}

before(() => {
  rejectSave = false
  savedRequests.length = 0
})

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

async function mountWorkbench(): Promise<{
  container: HTMLElement
  root: Root
  queryClient: QueryClient
}> {
  const domContainer = browserWindow.document.createElement('div')
  const container = domContainer as unknown as HTMLElement
  browserWindow.document.body.append(domContainer)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <RouterContextProvider router={router}>
          <QueryClientProvider client={queryClient}>
            <ImagePricingWorkbench catalog={catalog} routing={routing} />
          </QueryClientProvider>
        </RouterContextProvider>
      </I18nextProvider>
    )
  })
  return { container, root, queryClient }
}

async function unmountWorkbench(mounted: {
  container: HTMLElement
  root: Root
  queryClient: QueryClient
}) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

function getSalePriceInput(container: HTMLElement): HTMLInputElement {
  const input = container.querySelector(
    'input[aria-label="gpt-image-2 gen-1024x1024-medium sale price"]'
  )
  assert.ok(input instanceof browserWindow.HTMLInputElement)
  return input
}

function getButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button
}

async function editPrice(input: HTMLInputElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    Object.getPrototypeOf(input),
    'value'
  )?.set
  assert.ok(valueSetter)
  await act(async () => {
    input.focus()
    valueSetter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function editJson(input: HTMLTextAreaElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    Object.getPrototypeOf(input),
    'value'
  )?.set
  assert.ok(valueSetter)
  await act(async () => {
    valueSetter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function waitForAssertion(assertion: () => void) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion()
      return
    } catch (error) {
      if (attempt === 19) throw error
      await new Promise<void>((resolve) => setImmediate(resolve))
    }
  }
}

test('counts only values that differ from their saved prices', () => {
  assert.equal(
    countUnsavedImagePricingChanges(
      { 'gpt-image-2|gen-1024x1024-medium': '0.035' },
      { 'gpt-image-2|gen-1024x1024-medium': '0.03' }
    ),
    1
  )
  assert.equal(
    countUnsavedImagePricingChanges(
      { 'gpt-image-2|gen-1024x1024-medium': '0.03' },
      { 'gpt-image-2|gen-1024x1024-medium': '0.03' }
    ),
    0
  )
})

test('saves an edited sale price and clears the dirty state', async () => {
  const mounted = await mountWorkbench()
  try {
    const input = getSalePriceInput(mounted.container)
    await editPrice(input, '0.035')
    assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*1/)

    await act(async () => getButton(mounted.container, 'Save changes').click())

    assert.equal(savedRequests.length, 1)
    assert.equal(savedRequests[0]?.key, 'ImageModelCatalog')
    const savedCatalog = JSON.parse(savedRequests[0]?.value ?? '{}') as {
      models: Record<
        string,
        { skus: Record<string, { sale_price_usd: string }> }
      >
    }
    assert.equal(
      savedCatalog.models['gpt-image-2']?.skus['gen-1024x1024-medium']
        ?.sale_price_usd,
      '0.035'
    )
    assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*0/)
    assert.equal(input.value, '0.035')
  } finally {
    await unmountWorkbench(mounted)
  }
})

test('failed save keeps the edited value and dirty state', async () => {
  rejectSave = true
  const mounted = await mountWorkbench()
  try {
    const input = getSalePriceInput(mounted.container)
    await editPrice(input, '0.035')
    await act(async () => getButton(mounted.container, 'Save changes').click())

    assert.equal(input.value, '0.035')
    assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*1/)
  } finally {
    rejectSave = false
    await unmountWorkbench(mounted)
  }
})

test('rejects an invalid price before sending a save request', async () => {
  const initialSaveCount = savedRequests.length
  const mounted = await mountWorkbench()
  try {
    const input = getSalePriceInput(mounted.container)
    await editPrice(input, '-0.01')
    await act(async () => getButton(mounted.container, 'Save changes').click())

    assert.equal(input.getAttribute('aria-invalid'), 'true')
    assert.match(
      mounted.container.textContent ?? '',
      /Price must be non-negative/
    )
    assert.equal(savedRequests.length, initialSaveCount)
  } finally {
    await unmountWorkbench(mounted)
  }
})

test('Escape restores the saved sale price without a dirty change', async () => {
  const mounted = await mountWorkbench()
  try {
    const input = getSalePriceInput(mounted.container)
    await editPrice(input, '0.035')
    await act(async () => {
      input.dispatchEvent(
        new KeyboardEvent('keydown', {
          bubbles: true,
          key: 'Escape',
        })
      )
    })

    assert.equal(input.value, '0.03')
    assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*0/)
  } finally {
    await unmountWorkbench(mounted)
  }
})

test('keeps advanced settings collapsed until opened and previews routing', async () => {
  const mounted = await mountWorkbench()
  try {
    assert.match(mounted.container.textContent ?? '', /Not configured|\$0\.02/)
    const advancedButton = getButton(mounted.container, 'Advanced settings')
    assert.equal(advancedButton.getAttribute('aria-expanded'), 'false')
    assert.equal(
      mounted.container.querySelector('textarea[name="image-catalog"]'),
      null
    )

    await act(async () => advancedButton.click())

    assert.equal(advancedButton.getAttribute('aria-expanded'), 'true')
    const catalogEditor = mounted.container.querySelector(
      'textarea[name="image-catalog"]'
    )
    assert.ok(catalogEditor instanceof browserWindow.HTMLTextAreaElement)

    await act(async () =>
      getButton(mounted.container, 'Preview image routing').click()
    )
    assert.match(mounted.container.textContent ?? '', /Routing preview/)
  } finally {
    await unmountWorkbench(mounted)
  }
})

test('normalizes advanced JSON after saving so dirty state clears', async () => {
  const initialSaveCount = savedRequests.length
  const mounted = await mountWorkbench()
  try {
    await act(async () =>
      getButton(mounted.container, 'Advanced settings').click()
    )
    const routingEditorNode = mounted.container.querySelector(
      'textarea[name="image-routing"]'
    )
    assert.ok(routingEditorNode)
    const routingEditor = routingEditorNode as HTMLTextAreaElement
    await editJson(
      routingEditor,
      '{"version":1,"default":{"strategy":"manual"},"groups":{}}'
    )

    assert.equal(routingEditor.value.includes('manual'), true)
    assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*1/)
    assert.equal(
      getButton(mounted.container, 'Save image settings').disabled,
      false
    )
    await act(async () => {
      getButton(mounted.container, 'Save image settings').click()
      await Promise.resolve()
      await Promise.resolve()
    })

    await waitForAssertion(() => {
      assert.match(mounted.container.textContent ?? '', /Unsaved changes:\s*0/)
      assert.equal(
        savedRequests
          .slice(initialSaveCount)
          .some((request) => request.key === 'ImageRoutingPolicy'),
        true
      )
    })
  } finally {
    await unmountWorkbench(mounted)
  }
})
