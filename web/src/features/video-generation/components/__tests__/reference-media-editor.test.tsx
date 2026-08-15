/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act, useState } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { VideoMedia } from '../../types'

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

const { createRoot } = await import('react-dom/client')
const { ReferenceMediaEditor } = await import('../reference-media-editor')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en' })

type MountedEditor = {
  container: HTMLElement
  root: Root
  changes: string[][]
}

async function mountEditor(options: {
  kind: keyof VideoMedia
  values: string[]
  limit: number
  disabled?: boolean
}): Promise<MountedEditor> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const changes: string[][] = []

  function Harness() {
    const [values, setValues] = useState(options.values)
    return (
      <ReferenceMediaEditor
        kind={options.kind}
        values={values}
        limit={options.limit}
        disabled={options.disabled}
        onChange={(next) => {
          changes.push(next)
          setValues(next)
        }}
      />
    )
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>
    )
  })
  return {
    container: container as unknown as HTMLElement,
    root,
    changes,
  }
}

async function unmountEditor(mounted: MountedEditor) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

test('uses the supplied media limit for counting and adding URLs', async () => {
  const mounted = await mountEditor({ kind: 'videos', values: [], limit: 1 })
  const addButton = [...mounted.container.querySelectorAll('button')].find(
    (button) => button.textContent?.includes('Add URL')
  )
  assert.ok(addButton)
  assert.match(mounted.container.textContent ?? '', /0 \/ 1/)

  await act(async () => addButton.click())

  assert.deepEqual(mounted.changes, [['']])
  assert.match(mounted.container.textContent ?? '', /1 \/ 1/)
  assert.equal(addButton.disabled, true)
  await unmountEditor(mounted)
})

test('disables all URL editing controls when the media kind is unavailable', async () => {
  const mounted = await mountEditor({
    kind: 'videos',
    values: ['https://example.com/reference.mp4'],
    limit: 3,
    disabled: true,
  })
  const section = mounted.container.querySelector('section')
  const input = mounted.container.querySelector('input')
  const buttons = mounted.container.querySelectorAll('button')

  assert.equal(section?.getAttribute('aria-disabled'), 'true')
  assert.equal(input?.disabled, true)
  assert.equal(buttons.length, 2)
  assert.equal(
    [...buttons].every((button) => button.disabled),
    true
  )
  await unmountEditor(mounted)
})

after(() => browserWindow.close())
