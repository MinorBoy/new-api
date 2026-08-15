/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act, useState } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { api } from '@/lib/api'

import type { VideoImageSource } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/video-generation' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
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

const originalAdapter = api.defaults.adapter
api.defaults.adapter = async (config) => ({
  data: {
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 12 },
  },
  status: 200,
  statusText: 'OK',
  headers: {},
  config,
})

const { createRoot } = await import('react-dom/client')
const { ReferenceImageEditor } = await import('../reference-image-editor')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en' })

type MountedEditor = {
  container: HTMLElement
  root: Root
  queryClient: QueryClient
  changes: VideoImageSource[]
  renderModel: (model: string) => Promise<void>
}

async function mountEditor(
  initialSource: VideoImageSource,
  initialModel = 'doubao-seedance-2-0-260128'
): Promise<MountedEditor> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const changes: VideoImageSource[] = []

  function Harness(props: { model: string }) {
    const [source, setSource] = useState<VideoImageSource>(initialSource)
    return (
      <ReferenceImageEditor
        model={props.model}
        source={source}
        imageUrls={[]}
        assetIds={[]}
        apiKeyId={7}
        apiKey='sk-seven'
        imageLimit={9}
        onSourceChange={(next) => {
          changes.push(next)
          setSource(next)
        }}
        onImageUrlsChange={() => undefined}
        onAssetIdsChange={() => undefined}
      />
    )
  }

  async function renderModel(model: string) {
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={queryClient}>
            <Harness model={model} />
          </QueryClientProvider>
        </I18nextProvider>
      )
    })
  }

  await renderModel(initialModel)
  return {
    container: container as unknown as HTMLElement,
    root,
    queryClient,
    changes,
    renderModel,
  }
}

async function waitForButton(container: HTMLElement, label: string) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    const button = [...container.querySelectorAll('button')].find((candidate) =>
      candidate.textContent?.includes(label)
    )
    if (button) return button
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail(`Timed out waiting for ${label}`)
}

async function waitForLink(container: HTMLElement, href: string) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    const link = container.querySelector(`a[href="${href}"]`)
    if (link) return link
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail(`Timed out waiting for link ${href}`)
}

async function unmountEditor(mounted: MountedEditor) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
  mounted.queryClient.clear()
}

test('switches from public URLs to the asset library with accessible state', async () => {
  const mounted = await mountEditor('url')
  const publicUrls = await waitForButton(mounted.container, 'Public URLs')
  const assetLibrary = await waitForButton(mounted.container, 'Asset library')

  assert.equal(publicUrls.getAttribute('aria-pressed'), 'true')
  assert.equal(assetLibrary.getAttribute('aria-pressed'), 'false')
  await act(async () => assetLibrary.focus())
  assert.equal(browserWindow.document.activeElement, assetLibrary)

  await act(async () => {
    assetLibrary.click()
    await new Promise<void>((resolve) => {
      browserWindow.setTimeout(resolve, 0)
    })
  })
  await waitForLink(mounted.container, '/assets')

  assert.deepEqual(mounted.changes, ['asset'])
  assert.equal(publicUrls.getAttribute('aria-pressed'), 'false')
  assert.equal(assetLibrary.getAttribute('aria-pressed'), 'true')
  await unmountEditor(mounted)
})

test('returns to public URLs when the model does not support role assets', async () => {
  const mounted = await mountEditor('asset')
  await waitForLink(mounted.container, '/assets')

  await mounted.renderModel('doubao-seedance-2-5-260628')
  const publicUrls = await waitForButton(mounted.container, 'Public URLs')
  const assetLibrary = await waitForButton(mounted.container, 'Asset library')

  assert.deepEqual(mounted.changes, ['url'])
  assert.equal(publicUrls.getAttribute('aria-pressed'), 'true')
  assert.equal(assetLibrary.disabled, true)
  assert.ok(await waitForButton(mounted.container, 'Add URL'))
  await unmountEditor(mounted)
})

test('keeps asset mode while the selected key model is loading', async () => {
  const mounted = await mountEditor('asset')

  await mounted.renderModel('')
  const assetLibrary = await waitForButton(mounted.container, 'Asset library')

  assert.equal(assetLibrary.getAttribute('aria-pressed'), 'true')
  assert.deepEqual(mounted.changes, [])
  await unmountEditor(mounted)
})

after(() => {
  api.defaults.adapter = originalAdapter
  browserWindow.close()
})
