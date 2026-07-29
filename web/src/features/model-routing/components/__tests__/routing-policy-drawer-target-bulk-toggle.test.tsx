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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { routingPolicyQueryKeys } from '../../query-keys'
import type { RoutingPolicy } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
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
Object.assign(globalThis as Record<string, unknown>, browserGlobals)

after(() => {
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
})

const { createRoot } = await import('react-dom/client')
const { RoutingPolicyDrawer } = await import('../routing-policy-drawer')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

function createPolicy(targetEnabled: boolean[]): RoutingPolicy {
  return {
    id: 1,
    group_name: 'default',
    model: 'doubao-seedance-2-0-260128',
    enabled: false,
    defaults: {
      output_resolution: '720p',
      duration_seconds: 10,
      aspect_ratio: '16:9',
    },
    targets: targetEnabled.map((enabled, index) => ({
      id: index + 1,
      channel_id: index + 11,
      channel_name: `channel-${index + 1}`,
      name: `target-${index + 1}`,
      upstream_model: `upstream-model-${index + 1}`,
      cost_variant_key: 'default',
      target_priority: index,
      minimum_expected_margin_bps: null,
      enabled,
      constraints: {
        output_resolutions: ['720p'],
        durations: { min: 4, max: 15 },
        aspect_ratios: [],
        input_modes: ['text'],
        reference_minimums: { images: 0, videos: 0, audios: 0 },
        reference_limits: { images: 9, videos: 3, audios: 3 },
        supports_real_person: null,
      },
    })),
    created_at: 1,
    updated_at: 1,
  }
}

function createQueryClient(policy: RoutingPolicy): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Number.POSITIVE_INFINITY,
        refetchOnMount: false,
      },
      mutations: { retry: false },
    },
  })
  queryClient.setQueryData(routingPolicyQueryKeys.groups(), {
    success: true,
    message: '',
    data: ['default'],
  })
  queryClient.setQueryData(
    routingPolicyQueryKeys.candidates(policy.group_name, policy.model),
    {
      success: true,
      message: '',
      data: policy.targets.map((target) => ({
        id: target.channel_id,
        name: target.channel_name,
        status: 1,
        priority: 0,
        weight: 1,
      })),
    }
  )
  return queryClient
}

async function mountDrawer(policy: RoutingPolicy): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClient
}> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = createQueryClient(policy)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <RoutingPolicyDrawer
            open
            editingPolicy={policy}
            copyingPolicy={null}
            onOpenChange={() => {}}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await act(async () => undefined)
  return { root, container, queryClient }
}

async function unmountDrawer(mounted: {
  root: Root
  container: { remove(): void }
  queryClient: QueryClient
}) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

function getBulkSwitch(): HTMLElement {
  const control = browserWindow.document.querySelector(
    '[data-slot="switch"][aria-label="Enable all targets"]'
  )
  assert.ok(control instanceof browserWindow.HTMLElement)
  return control as unknown as HTMLElement
}

function getPolicyAndTargetSwitches(): HTMLElement[] {
  return [
    ...browserWindow.document.querySelectorAll(
      '[data-slot="form-control"][aria-checked]'
    ),
  ] as unknown as HTMLElement[]
}

function findButton(label: string): HTMLButtonElement {
  const button = [...browserWindow.document.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

function switchChecked(control: HTMLElement): boolean {
  return control.getAttribute('aria-checked') === 'true'
}

async function expandAllTargetEditors() {
  const triggers = [
    ...browserWindow.document.querySelectorAll(
      '[data-slot="accordion-trigger"]'
    ),
  ]
  for (const trigger of triggers) {
    if (trigger.getAttribute('aria-expanded') !== 'true') {
      await act(async () => {
        const element = trigger as unknown as HTMLElement
        element.click()
      })
    }
  }
}

test('mixed targets render bulk off and bulk changes preserve policy enablement', async () => {
  const mounted = await mountDrawer(createPolicy([true, false]))
  try {
    await expandAllTargetEditors()
    const bulkSwitch = getBulkSwitch()
    const enabledSwitches = getPolicyAndTargetSwitches()
    assert.equal(switchChecked(bulkSwitch), false)
    assert.deepEqual(enabledSwitches.map(switchChecked), [false, true, false])

    await act(async () => bulkSwitch.click())
    assert.deepEqual(getPolicyAndTargetSwitches().map(switchChecked), [
      false,
      true,
      true,
    ])

    await act(async () => bulkSwitch.click())
    assert.deepEqual(getPolicyAndTargetSwitches().map(switchChecked), [
      false,
      false,
      false,
    ])
  } finally {
    await unmountDrawer(mounted)
  }
})

test('adding the default disabled target turns an all-enabled bulk switch off', async () => {
  const mounted = await mountDrawer(createPolicy([true, true]))
  try {
    const bulkSwitch = getBulkSwitch()
    assert.equal(switchChecked(bulkSwitch), true)

    await act(async () => findButton('Add target').click())
    await expandAllTargetEditors()

    assert.equal(switchChecked(bulkSwitch), false)
    assert.deepEqual(getPolicyAndTargetSwitches().map(switchChecked), [
      false,
      true,
      true,
      false,
    ])
  } finally {
    await unmountDrawer(mounted)
  }
})

test('an empty target list disables the bulk switch', async () => {
  const mounted = await mountDrawer(createPolicy([]))
  try {
    const bulkSwitch = getBulkSwitch()
    assert.equal(
      bulkSwitch.hasAttribute('disabled') ||
        bulkSwitch.getAttribute('aria-disabled') === 'true',
      true
    )
    assert.equal(switchChecked(bulkSwitch), false)
  } finally {
    await unmountDrawer(mounted)
  }
})

test('routing targets start collapsed and expand from their name headings', async () => {
  const mounted = await mountDrawer(createPolicy([true, false]))
  try {
    const triggers = [
      ...browserWindow.document.querySelectorAll(
        '[data-slot="accordion-trigger"]'
      ),
    ]
    assert.deepEqual(
      triggers.map((trigger) => trigger.textContent?.trim()),
      ['target-1', 'target-2']
    )
    assert.deepEqual(
      triggers.map((trigger) => trigger.getAttribute('aria-expanded')),
      ['false', 'false']
    )

    const firstTrigger = triggers[0]
    assert.ok(firstTrigger instanceof browserWindow.HTMLElement)
    await act(async () => firstTrigger.click())
    assert.equal(firstTrigger.getAttribute('aria-expanded'), 'true')
    const upstreamInput = [
      ...browserWindow.document.querySelectorAll('input'),
    ].find(
      (input) =>
        (input as unknown as { value: string }).value === 'upstream-model-1'
    )
    assert.ok(upstreamInput)
  } finally {
    await unmountDrawer(mounted)
  }
})
