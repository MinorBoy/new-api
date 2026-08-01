// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
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

// No `docs_link` configured → the docs entry must point at the internal
// `/docs` platform and request a new tab.
mock.module('@/hooks/use-status', () => ({
  useStatus: () => ({ status: {} }),
}))
mock.module('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: {} }),
}))

const { createRoot } = await import('react-dom/client')
const { useTopNavLinks } = await import('../use-top-nav-links')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: { translation: { Home: 'Home', Console: 'Console', Docs: 'Docs' } },
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

test('opens the internal docs platform in a new tab when no docs_link is configured', async () => {
  const mounted = await mountNavigationLinks()
  const links = JSON.parse(
    mounted.container.querySelector('output')?.textContent ?? '[]'
  ) as Array<{
    title: string
    href: string
    external?: boolean
    openInNewTab?: boolean
  }>

  const docs = links.find((link) => link.title === 'Docs')
  assert.ok(docs, 'a Docs entry should be present')
  assert.equal(docs.href, '/docs')
  assert.equal(docs.external, undefined)
  assert.equal(docs.openInNewTab, true)

  await unmountNavigationLinks(mounted)
})
