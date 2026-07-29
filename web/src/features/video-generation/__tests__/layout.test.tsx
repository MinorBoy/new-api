import assert from 'node:assert/strict'
import test, { after } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

const browserWindow = new Window({ url: 'http://localhost/video-generation' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.assign(globalThis as Record<string, unknown>, {
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
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const { createRoot } = await import('react-dom/client')
const { VideoGeneration } = await import('../index')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

test('renders labeled key and model selectors and blocks submit without a key', async () => {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root: Root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <VideoGeneration />
        </QueryClientProvider>
      </I18nextProvider>
    )
    await Promise.resolve()
  })

  const keySelect = browserWindow.document.querySelector(
    'select[aria-label="API key"]'
  )
  const modelSelect = browserWindow.document.querySelector(
    'select[aria-label="Model"]'
  )
  const submit = browserWindow.document.querySelector('button[type="submit"]')
  assert.ok(keySelect)
  assert.ok(modelSelect)
  assert.equal((submit as HTMLButtonElement | null)?.disabled, true)

  await act(async () => root.unmount())
  container.remove()
})
