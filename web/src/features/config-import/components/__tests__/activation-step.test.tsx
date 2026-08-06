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

import type {
  ConfigImportActivationPreview,
  ConfigImportBatchDetail,
} from '../../types'
import { ActivationStep } from '../activation-step'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())
beforeEach(() => browserWindow.document.body.replaceChildren())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

function batch(
  activationPreview: ConfigImportActivationPreview
): ConfigImportBatchDetail {
  return {
    id: 12,
    schema_version: 1,
    template_version: 'v1',
    source_sha256: 'source',
    payload_sha256: 'payload',
    status: 'published',
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
    allowed_actions: ['activate'],
    activated_at: null,
    created_at: 1,
    updated_at: 1,
    items: [],
    issues: [],
    activation_preview: activationPreview,
  }
}

async function mount(
  preview: ConfigImportActivationPreview,
  options: {
    canActivate?: boolean
    isActivating?: boolean
    onActivate?: () => Promise<void>
  } = {}
) {
  let calls = 0
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ActivationStep
          batch={batch(preview)}
          canActivate={options.canActivate ?? true}
          isActivating={options.isActivating}
          onActivate={async () => {
            calls += 1
            await options.onActivate?.()
          }}
        />
      </I18nextProvider>
    )
  })
  return { container, root, calls: () => calls }
}

function button(container: HappyHTMLElement, text: string): HTMLButtonElement {
  const value = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === text
  ) as HTMLButtonElement | undefined
  assert.ok(value)
  return value
}

function checkbox(
  container: HappyHTMLElement,
  label: string
): HTMLInputElement {
  const value = container.querySelector(
    `input[type="checkbox"][aria-label="${label}"]`
  ) as HTMLInputElement | null
  assert.ok(value)
  return value
}

test('requires a ready preview and explicit confirmation', async () => {
  const mounted = await mount({
    ready: true,
    channel_count: 2,
    policy_count: 3,
    target_count: 13,
    retire_target_count: 67,
    blockers: [],
  })
  try {
    const activate = button(mounted.container, 'Activate import')
    assert.equal(activate.disabled, true)
    await act(async () =>
      checkbox(mounted.container, 'Confirm activation').click()
    )
    assert.equal(activate.disabled, false)
    await act(async () => activate.click())
    assert.equal(mounted.calls(), 1)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('shows every blocker and keeps activation disabled', async () => {
  const mounted = await mount({
    ready: false,
    channel_count: 1,
    policy_count: 1,
    target_count: 1,
    retire_target_count: 0,
    blockers: [
      {
        code: 'ACTIVATION_COST_DRAFT_MISSING',
        message: 'Missing cost draft.',
        route_target_ref: 'route-a',
      },
      {
        code: 'ACTIVATION_CHANNEL_AUTO_DISABLED',
        message: 'Channel is auto disabled.',
        channel_id: 9,
      },
    ],
  })
  try {
    const content = mounted.container.textContent ?? ''
    assert.match(content, /ACTIVATION_COST_DRAFT_MISSING/)
    assert.match(content, /route-a/)
    assert.match(content, /ACTIVATION_CHANNEL_AUTO_DISABLED/)
    assert.match(content, /9/)
    assert.equal(button(mounted.container, 'Activate import').disabled, true)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('shows stable activation counts', async () => {
  const mounted = await mount({
    ready: true,
    channel_count: 2,
    policy_count: 3,
    target_count: 13,
    retire_target_count: 67,
    blockers: [],
  })
  try {
    const content = mounted.container.textContent ?? ''
    assert.match(content, /Channels to enable2/)
    assert.match(content, /Policies to enable3/)
    assert.match(content, /Targets to enable13/)
    assert.match(content, /Targets to retire67/)
    assert.equal(
      mounted.container.querySelectorAll('[data-activation-count]').length,
      4
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('keeps activation disabled while the request is pending', async () => {
  let resolveActivation: (() => void) | undefined
  const pending = new Promise<void>((resolve) => {
    resolveActivation = resolve
  })
  const mounted = await mount(
    {
      ready: true,
      channel_count: 1,
      policy_count: 1,
      target_count: 1,
      retire_target_count: 0,
      blockers: [],
    },
    { onActivate: () => pending }
  )
  try {
    await act(async () =>
      checkbox(mounted.container, 'Confirm activation').click()
    )
    const activate = button(mounted.container, 'Activate import')
    await act(async () => {
      activate.click()
      await Promise.resolve()
    })
    assert.equal(activate.disabled, true)
    await act(async () => resolveActivation?.())
    assert.equal(activate.disabled, false)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('shows a backend activation error as an alert', async () => {
  const mounted = await mount(
    {
      ready: true,
      channel_count: 1,
      policy_count: 1,
      target_count: 1,
      retire_target_count: 0,
      blockers: [],
    },
    {
      onActivate: async () => {
        throw new Error('upstream rejected')
      },
    }
  )
  try {
    await act(async () =>
      checkbox(mounted.container, 'Confirm activation').click()
    )
    await act(async () => button(mounted.container, 'Activate import').click())
    const alert = mounted.container.querySelector('[role="alert"]')
    assert.ok(alert)
    assert.match(alert.textContent ?? '', /upstream rejected/)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('wraps long route target references inside blocker details', async () => {
  const mounted = await mount({
    ready: false,
    channel_count: 1,
    policy_count: 1,
    target_count: 1,
    retire_target_count: 0,
    blockers: [
      {
        code: 'ACTIVATION_TARGET_MISSING',
        message: 'Target is missing.',
        route_target_ref: 'route-target-with-a-very-long-unbroken-reference',
      },
    ],
  })
  try {
    const reference = mounted.container.querySelector('[data-route-target-ref]')
    assert.ok(reference)
    assert.equal(reference.classList.contains('break-all'), true)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
