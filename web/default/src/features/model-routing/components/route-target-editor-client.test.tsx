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
import test, { after } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { FormProvider, useForm, type UseFormReturn } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'

import {
  createEmptyPolicyForm,
  createEmptyTarget,
  type RouteTargetFormValues,
  type RoutingPolicyFormValues,
} from '../types'
import { RouteTargetEditor } from './route-target-editor'

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

const { createRoot } = await import('react-dom/client')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

type FormRef = {
  current?: UseFormReturn<RoutingPolicyFormValues>
}

function TargetEditorFixture(props: {
  formRef: FormRef
  target: RouteTargetFormValues
}) {
  const policy = createEmptyPolicyForm()
  const form = useForm<RoutingPolicyFormValues>({
    defaultValues: { ...policy, targets: [props.target] },
  })
  props.formRef.current = form

  return (
    <I18nextProvider i18n={i18n}>
      <FormProvider {...form}>
        <RouteTargetEditor
          form={form}
          index={0}
          candidates={[]}
          candidatesLoading={false}
          canRemove
          onCopy={() => {}}
          onRemove={() => {}}
        />
      </FormProvider>
    </I18nextProvider>
  )
}

async function mountTargetEditor(target = createEmptyTarget()): Promise<{
  form: UseFormReturn<RoutingPolicyFormValues>
  root: Root
  container: { remove(): void }
}> {
  const formRef: FormRef = {}
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(<TargetEditorFixture formRef={formRef} target={target} />)
  })
  assert.ok(formRef.current)
  return { form: formRef.current, root, container }
}

async function unmountTargetEditor(root: Root, container: { remove(): void }) {
  await act(async () => root.unmount())
  container.remove()
}

test('generates and refreshes a routing target name from watched fields', async () => {
  const mounted = await mountTargetEditor()
  try {
    await act(async () => {
      mounted.form.setValue('targets.0.channel_name', 'A1')
    })
    assert.match(
      mounted.form.getValues('targets.0.name'),
      /^\d{8}-A1-720p-standard-4-15s$/
    )

    await act(async () => {
      mounted.form.setValue('targets.0.output_resolutions', ['1080p', '720p'])
    })
    assert.match(
      mounted.form.getValues('targets.0.name'),
      /^\d{8}-A1-720p\+1080p-standard-4-15s$/
    )
  } finally {
    await unmountTargetEditor(mounted.root, mounted.container)
  }
})

test('preserves a manual name and resumes generation after it is cleared', async () => {
  const mounted = await mountTargetEditor()
  try {
    await act(async () => {
      mounted.form.setValue('targets.0.channel_name', 'A1')
    })
    assert.match(
      mounted.form.getValues('targets.0.name'),
      /^\d{8}-A1-720p-standard-4-15s$/
    )

    await act(async () => {
      mounted.form.setValue('targets.0.name', 'manual target')
    })
    assert.equal(mounted.form.getValues('targets.0.name'), 'manual target')

    await act(async () => {
      mounted.form.setValue('targets.0.output_resolutions', ['1080p'])
    })
    assert.equal(mounted.form.getValues('targets.0.name'), 'manual target')

    await act(async () => {
      mounted.form.setValue('targets.0.name', '  ')
    })
    assert.match(
      mounted.form.getValues('targets.0.name'),
      /^\d{8}-A1-1080p-standard-4-15s$/
    )
  } finally {
    await unmountTargetEditor(mounted.root, mounted.container)
  }
})

test('does not replace a non-empty name when editing or copying a target', async () => {
  const target = createEmptyTarget()
  target.channel_name = 'A1'
  target.name = 'existing target copy'
  const mounted = await mountTargetEditor(target)
  try {
    assert.equal(
      mounted.form.getValues('targets.0.name'),
      'existing target copy'
    )
  } finally {
    await unmountTargetEditor(mounted.root, mounted.container)
  }
})
