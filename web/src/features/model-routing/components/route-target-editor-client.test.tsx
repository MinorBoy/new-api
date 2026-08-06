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
  copyPolicyForm,
  copyTargetForm,
  createEmptyPolicyForm,
  createEmptyTarget,
  fromPolicyResponse,
  routeTargetFormSchema,
  toWriteRequest,
  type RouteTargetFormValues,
  type RoutingPolicy,
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

test('routing target margin keeps null inheritance and explicit zero', () => {
  const inherited = createEmptyTarget()
  assert.equal(inherited.minimum_expected_margin_bps, null)

  const policy = createEmptyPolicyForm()
  const request = toWriteRequest({
    ...policy,
    group_name: 'default',
    enabled: true,
    targets: [
      {
        ...inherited,
        channel_id: 1,
        name: 'target',
        upstream_model: 'vendor-model',
        minimum_expected_margin_bps: 0,
      },
    ],
  })

  assert.equal(request.targets[0]?.minimum_expected_margin_bps, 0)
  assert.equal(
    routeTargetFormSchema.safeParse({
      ...inherited,
      channel_id: 1,
      name: 'target',
      upstream_model: 'vendor-model',
      minimum_expected_margin_bps: null,
    }).success,
    true
  )
  assert.equal(
    routeTargetFormSchema.safeParse({
      ...inherited,
      channel_id: 1,
      name: 'target',
      upstream_model: 'vendor-model',
      minimum_expected_margin_bps: -1,
    }).success,
    false
  )
  assert.equal(
    routeTargetFormSchema.safeParse({
      ...inherited,
      channel_id: 1,
      name: 'target',
      upstream_model: 'vendor-model',
      minimum_expected_margin_bps: 10001,
    }).success,
    false
  )
})

test('empty routing targets default to the default cost variant', () => {
  const target = createEmptyTarget() as unknown as Record<string, unknown>

  assert.equal(target.cost_variant_key, 'default')
})

test('routing target cost variants normalize valid keys and reject invalid keys', () => {
  const target = {
    ...createEmptyTarget(),
    channel_id: 1,
    name: 'target',
    upstream_model: 'vendor-model',
  }

  const parsed = routeTargetFormSchema.parse({
    ...target,
    cost_variant_key: ' 720P ',
  }) as unknown as Record<string, unknown>
  assert.equal(parsed.cost_variant_key, '720p')
  assert.equal(
    routeTargetFormSchema.safeParse({
      ...target,
      cost_variant_key: 'not a variant',
    }).success,
    false
  )
})

test('routing target forms normalize blank cost variants to default', () => {
  const policy = createEmptyPolicyForm()
  for (const costVariantKey of ['', ' \t ']) {
    const target: RouteTargetFormValues = {
      ...createEmptyTarget(),
      channel_id: 1,
      name: 'target',
      upstream_model: 'vendor-model',
      cost_variant_key: costVariantKey,
    }

    const parsed = routeTargetFormSchema.parse(target)
    assert.equal(parsed.cost_variant_key, 'default')

    const request = toWriteRequest({
      ...policy,
      group_name: 'default',
      enabled: true,
      targets: [target],
    })
    assert.equal(request.targets[0]?.cost_variant_key, 'default')
  }
})

test('routing target cost variants survive write and response mappings', () => {
  const target = createEmptyTarget() as unknown as Record<string, unknown>
  target.channel_id = 1
  target.name = 'target'
  target.upstream_model = 'vendor-model'
  target.cost_variant_key = ' 720P '
  const policy = createEmptyPolicyForm()
  const request = toWriteRequest({
    ...policy,
    group_name: 'default',
    enabled: true,
    targets: [target as unknown as RouteTargetFormValues],
  })
  const requestTarget = request.targets[0]
  assert.ok(requestTarget)
  assert.equal(
    (requestTarget as unknown as Record<string, unknown>).cost_variant_key,
    '720p'
  )

  const form = fromPolicyResponse({
    id: 1,
    group_name: 'default',
    model: policy.model,
    enabled: true,
    defaults: policy.defaults,
    targets: [
      {
        ...requestTarget,
        id: 1,
        channel_name: 'A1',
        cost_variant_key: '720p',
      },
    ],
    created_at: 1,
    updated_at: 1,
  } as unknown as RoutingPolicy)

  assert.equal(
    (form.targets[0] as unknown as Record<string, unknown>).cost_variant_key,
    '720p'
  )
})

test('routing target copies retain their cost variant', () => {
  const target: RouteTargetFormValues = {
    ...createEmptyTarget(),
    channel_id: 1,
    channel_name: 'A1',
    name: 'target',
    upstream_model: 'vendor-model',
    cost_variant_key: '720p',
  }
  const copiedTarget = copyTargetForm(target)
  assert.equal(copiedTarget.cost_variant_key, '720p')

  const policy = createEmptyPolicyForm()
  const request = toWriteRequest({
    ...policy,
    group_name: 'default',
    enabled: true,
    targets: [target],
  })
  const requestTarget = request.targets[0]
  assert.ok(requestTarget)
  const copiedPolicy = copyPolicyForm({
    id: 1,
    group_name: 'default',
    model: policy.model,
    enabled: true,
    defaults: policy.defaults,
    targets: [
      {
        ...requestTarget,
        id: 1,
        channel_name: 'A1',
      },
    ],
    created_at: 1,
    updated_at: 1,
  } as RoutingPolicy)

  assert.equal(copiedPolicy.targets[0]?.cost_variant_key, '720p')
})

test('editing preserves the target id while copying removes it', () => {
  const policy = createEmptyPolicyForm()
  const target: RouteTargetFormValues = {
    ...createEmptyTarget(),
    id: 42,
    channel_id: 1,
    channel_name: 'A1',
    name: 'target',
    upstream_model: 'vendor-model',
  }

  const edited = toWriteRequest({
    ...policy,
    group_name: 'default',
    enabled: true,
    targets: [target],
  })
  assert.equal(edited.targets[0]?.id, 42)

  const copied = copyTargetForm(target)
  const copiedRequest = toWriteRequest({
    ...policy,
    group_name: 'default',
    enabled: true,
    targets: [copied],
  })
  assert.equal(copied.id, undefined)
  assert.equal(copiedRequest.targets[0]?.id, undefined)
})

test('edits input modes and reference minimums in the submitted constraints', async () => {
  const mounted = await mountTargetEditor()
  try {
    const buttons = [...browserWindow.document.querySelectorAll('button')]
    const textMode = buttons.find(
      (button) => button.textContent?.trim() === 'Text to video'
    )
    const framePairMode = buttons.find(
      (button) => button.textContent?.trim() === 'First and last frames'
    )
    assert.ok(textMode)
    assert.ok(framePairMode)

    await act(async () => {
      textMode.click()
    })
    await act(async () => {
      framePairMode.click()
    })

    const minimumImagesLabel = [
      ...browserWindow.document.querySelectorAll('label'),
    ].find((label) => label.textContent?.trim() === 'Minimum reference images')
    assert.ok(minimumImagesLabel instanceof browserWindow.HTMLLabelElement)
    const minimumImages = browserWindow.document.querySelector(
      `input[id="${minimumImagesLabel.htmlFor}"]`
    )
    assert.ok(minimumImages instanceof browserWindow.HTMLInputElement)
    const inputValueSetter = Object.getOwnPropertyDescriptor(
      Object.getPrototypeOf(minimumImages) as object,
      'value'
    )?.set
    assert.ok(inputValueSetter)
    const eventTarget = minimumImages as unknown as {
      dispatchEvent: (event: unknown) => boolean
    }

    await act(async () => {
      inputValueSetter.call(minimumImages, '1')
      eventTarget.dispatchEvent(
        new browserWindow.Event('input', { bubbles: true })
      )
    })

    assert.deepEqual(mounted.form.getValues('targets.0.input_modes'), [
      'first_frame',
      'omni_reference',
    ])
    assert.deepEqual(mounted.form.getValues('targets.0.reference_minimums'), {
      images: 1,
      videos: 0,
      audios: 0,
    })

    const policy = mounted.form.getValues()
    policy.group_name = 'discount'
    policy.enabled = true
    const policyTarget = policy.targets[0]
    assert.ok(policyTarget)
    policyTarget.channel_id = 1
    policyTarget.name = 'discount target'
    policyTarget.upstream_model = 'provider-model'
    const payload = toWriteRequest(policy)

    assert.deepEqual(payload.targets[0]?.constraints.input_modes, [
      'first_frame',
      'omni_reference',
    ])
    assert.deepEqual(payload.targets[0]?.constraints.reference_minimums, {
      images: 1,
      videos: 0,
      audios: 0,
    })
  } finally {
    await unmountTargetEditor(mounted.root, mounted.container)
  }
})
