/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { after, test } from 'node:test'

import { Window } from 'happy-dom'

import type { UsageLog } from '../../data/schema'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

Object.defineProperty(domWindow, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
})
Object.defineProperty(globalThis, 'matchMedia', {
  configurable: true,
  value: domWindow.matchMedia,
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { DetailsDialog } = await import('../dialogs/details-dialog')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

test('regular log details never render injected supplier fields', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const log: UsageLog = {
    id: 1,
    user_id: 10,
    created_at: 1710000000,
    type: 2,
    content: 'completed',
    username: 'alice',
    token_name: 'user-token',
    model_name: 'public-model',
    quota: 125000,
    prompt_tokens: 10,
    completion_tokens: 20,
    use_time: 3,
    is_stream: false,
    channel: 40,
    channel_name: 'supplier-channel',
    token_id: 7,
    group: 'internal-group',
    ip: '10.0.0.8',
    request_id: 'req-public',
    upstream_request_id: 'upstream-secret',
    other: JSON.stringify({
      is_model_mapped: true,
      upstream_model_name: 'provider-model',
      model_price: 0.25,
      group_ratio: 1.25,
      request_path: '/supplier/private/path',
      admin_info: {
        use_channel: [40, 41],
        cost_accounting_request_id: 91,
      },
    }),
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <DetailsDialog log={log} isAdmin={false} open onOpenChange={() => {}} />
      </I18nextProvider>
    )
  })

  const text = document.body.textContent ?? ''
  for (const forbidden of [
    'supplier-channel',
    'upstream-secret',
    'internal-group',
    'provider-model',
    'Upstream Request ID',
    'Model Mapping',
    'Retry Chain',
    'Supplier Cost Accounting',
  ]) {
    assert.equal(text.includes(forbidden), false, forbidden)
  }
  assert.match(text, /req-public/)
  assert.match(text, /public-model/)

  await act(async () => root.unmount())
  container.remove()
})

after(() => {
  domWindow.close()
})
