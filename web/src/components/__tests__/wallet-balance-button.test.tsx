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

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

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
const { WalletBalanceButton } = await import('../wallet-balance-button')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: {
      translation: {
        'Current Balance': 'Current Balance',
      },
    },
  },
})

async function mountButton(props: {
  quota?: number
  loading?: boolean
  onOpenWallet?: () => void
}) {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <WalletBalanceButton
          quota={props.quota}
          loading={props.loading ?? false}
          onOpenWallet={props.onOpenWallet ?? (() => {})}
        />
      </I18nextProvider>
    )
  })

  return { container: container as unknown as HTMLElement, root }
}

async function unmountButton(mounted: { root: Root; container: HTMLElement }) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

function getBalanceButton(container: HTMLElement) {
  const button = container.querySelector(
    '[data-testid="wallet-balance-button"]'
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

test('renders the configured display balance and opens the wallet', async () => {
  let openWalletCount = 0
  const mounted = await mountButton({
    quota: 6_250_000,
    onOpenWallet: () => {
      openWalletCount += 1
    },
  })

  const button = getBalanceButton(mounted.container)
  assert.match(button.textContent ?? '', /\$12\.5/)
  assert.match(button.getAttribute('aria-label') ?? '', /Current Balance.*\$12\.5/)

  await act(async () => {
    button.click()
  })
  assert.equal(openWalletCount, 1)

  await unmountButton(mounted)
})

test('reserves the amount slot while the balance is loading', async () => {
  const mounted = await mountButton({ loading: true })

  const button = getBalanceButton(mounted.container)
  assert.ok(
    button.querySelector('[data-testid="wallet-balance-loading"]') !== null
  )
  assert.equal(button.getAttribute('aria-label'), 'Current Balance')

  await unmountButton(mounted)
})

test('keeps an icon-only wallet action when a balance is unavailable', async () => {
  const mounted = await mountButton({})

  const button = getBalanceButton(mounted.container)
  assert.equal(button.querySelector('[data-testid="wallet-balance-value"]'), null)
  assert.equal(button.getAttribute('aria-label'), 'Current Balance')

  await unmountButton(mounted)
})
