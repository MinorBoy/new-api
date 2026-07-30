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

// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
import { Window } from 'happy-dom'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'
import { createInstance } from 'i18next'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
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

mock.module('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      docs_link: 'https://docs.example.com',
      HeaderNavModules: JSON.stringify({ about: true }),
    },
  }),
}))
mock.module('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: { id: 1 } } }),
}))

const { createRoot } = await import('react-dom/client')
const { useTopNavLinks } = await import('../use-top-nav-links')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: {
      translation: {
        Home: 'Home',
        Console: 'Console',
        Docs: 'Docs',
        About: 'About',
      },
    },
  },
})

function NavigationLinks() {
  return <output>{JSON.stringify(useTopNavLinks())}</output>
}

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

async function mountNavigationLinks() {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <NavigationLinks />
      </I18nextProvider>
    )
  })

  return { container: container as unknown as HTMLElement, root }
}

async function unmountNavigationLinks(mounted: {
  root: Root
  container: HTMLElement
}) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

test('omits the About entry even when legacy header configuration enables it', async () => {
  const mounted = await mountNavigationLinks()
  const links = JSON.parse(
    mounted.container.querySelector('output')?.textContent ?? '[]'
  ) as Array<{ href: string }>

  assert.equal(links.some((link) => link.href === '/about'), false)

  await unmountNavigationLinks(mounted)
})
