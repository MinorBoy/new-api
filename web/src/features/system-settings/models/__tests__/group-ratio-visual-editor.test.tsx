import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import type { QueryClient as QueryClientType } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  customElements: browserWindow.customElements,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  matchMedia: browserWindow.matchMedia.bind(browserWindow),
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame:
    browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
for (const [key, value] of Object.entries(browserGlobals)) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    writable: true,
    value,
  })
}

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { I18nextProvider } = await import('react-i18next')
const { GroupRatioVisualEditor } =
  await import('../group-ratio-visual-editor')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

type Values = {
  GroupRatio: string
  GroupStatus: string
  TopupGroupRatio: string
  UserUsableGroups: string
  GroupGroupRatio: string
  AutoGroups: string
  GroupSpecialUsableGroup: string
  GroupRoutingRequirements: string
}

const initialValues: Values = {
  GroupRatio: '{"default":1,"premium":2}',
  GroupStatus: '{"default":true,"premium":true}',
  TopupGroupRatio: '{"premium":1.1}',
  UserUsableGroups: '{"premium":"Premium"}',
  GroupGroupRatio: '{"premium":{"premium":2},"default":{"premium":1.5}}',
  AutoGroups: '["premium"]',
  GroupSpecialUsableGroup:
    '{"premium":{"premium":"Self"},"default":{"+:premium":"Premium"}}',
  GroupRoutingRequirements: '{"premium":{"require_real_person":true}}',
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

function findButton(name: string): HTMLButtonElement {
  const button = [...browserWindow.document.querySelectorAll('button')].find(
    (candidate) =>
      candidate.getAttribute('aria-label') === name ||
      candidate.textContent?.trim() === name
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

function findSwitch(name: string): HTMLElement {
  const control = browserWindow.document.querySelector(
    `[role="switch"][aria-label="${name}"]`
  )
  assert.ok(control instanceof browserWindow.HTMLElement)
  return control as unknown as HTMLElement
}

async function mountEditor(): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClientType
  getValues: () => Values
}> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  let latestValues = initialValues

  function Fixture() {
    const [values, setValues] = useState(initialValues)
    latestValues = values
    return (
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <GroupRatioVisualEditor
            groupRatio={values.GroupRatio}
            groupStatus={values.GroupStatus}
            topupGroupRatio={values.TopupGroupRatio}
            userUsableGroups={values.UserUsableGroups}
            groupGroupRatio={values.GroupGroupRatio}
            autoGroups={values.AutoGroups}
            groupSpecialUsableGroup={values.GroupSpecialUsableGroup}
            groupRoutingRequirements={values.GroupRoutingRequirements}
            onChange={(field, value) => {
              setValues((current) => ({ ...current, [field]: value }))
            }}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
  }

  await act(async () => root.render(<Fixture />))
  return { root, container, queryClient, getValues: () => latestValues }
}

async function unmountEditor(mounted: Awaited<ReturnType<typeof mountEditor>>) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

test('new groups are explicitly enabled and ordinary groups can be toggled', async () => {
  const mounted = await mountEditor()
  try {
    await act(async () => findButton('Add group').click())
    assert.equal(JSON.parse(mounted.getValues().GroupStatus).group_1, true)

    await act(async () => findSwitch('Toggle group group_1').click())
    assert.equal(JSON.parse(mounted.getValues().GroupStatus).group_1, false)
  } finally {
    await unmountEditor(mounted)
  }
})

test('disabling default requires confirmation and cancel preserves its state', async () => {
  const mounted = await mountEditor()
  try {
    const defaultSwitch = findSwitch('Toggle group default')
    await act(async () => defaultSwitch.click())
    assert.match(document.body.textContent ?? '', /Disable default group\?/)
    assert.equal(JSON.parse(mounted.getValues().GroupStatus).default, true)

    await act(async () => findButton('Cancel').click())
    assert.equal(JSON.parse(mounted.getValues().GroupStatus).default, true)

    await act(async () => defaultSwitch.click())
    await act(async () => findButton('Continue').click())
    assert.equal(JSON.parse(mounted.getValues().GroupStatus).default, false)
  } finally {
    await unmountEditor(mounted)
  }
})

test('deletion waits for confirmation and removes every owned configuration', async () => {
  const mounted = await mountEditor()
  try {
    await act(async () => findButton('Delete group premium').click())
    assert.match(document.body.textContent ?? '', /Delete group/)
    assert.equal(JSON.parse(mounted.getValues().GroupRatio).premium, 2)

    await act(async () => findButton('Cancel').click())
    assert.equal(JSON.parse(mounted.getValues().GroupRatio).premium, 2)

    await act(async () => findButton('Delete group premium').click())
    await act(async () => findButton('Confirm delete').click())

    const values = mounted.getValues()
    assert.equal(Object.hasOwn(JSON.parse(values.GroupRatio), 'premium'), false)
    assert.equal(Object.hasOwn(JSON.parse(values.GroupStatus), 'premium'), false)
    assert.equal(
      Object.hasOwn(JSON.parse(values.TopupGroupRatio), 'premium'),
      false
    )
    assert.equal(
      Object.hasOwn(JSON.parse(values.UserUsableGroups), 'premium'),
      false
    )
    assert.deepEqual(JSON.parse(values.GroupGroupRatio), { default: {} })
    assert.deepEqual(JSON.parse(values.AutoGroups), [])
    assert.deepEqual(JSON.parse(values.GroupSpecialUsableGroup), {
      default: {},
    })
    assert.equal(
      Object.hasOwn(JSON.parse(values.GroupRoutingRequirements), 'premium'),
      false
    )
  } finally {
    await unmountEditor(mounted)
  }
})
