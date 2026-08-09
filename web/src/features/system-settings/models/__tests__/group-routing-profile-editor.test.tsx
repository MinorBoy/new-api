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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { GroupRoutingProfileSummary } from '../group-routing-profile-api'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { GroupRoutingProfileEditor } =
  await import('../group-routing-profile-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const matchedSummary: GroupRoutingProfileSummary = {
  models: 2,
  matched_models: 2,
  targets: 3,
  matched_targets: 3,
  stale_exclusions: 0,
}

function findControl(container: HTMLElement, name: string): HTMLElement {
  const control = [
    ...container.querySelectorAll<HTMLElement>(
      'button, [role="switch"], [role="checkbox"]'
    ),
  ].find(
    (candidate) =>
      candidate.getAttribute('aria-label') === name ||
      candidate.textContent?.trim() === name
  )
  assert.ok(control, `Expected a control named ${name}`)
  return control
}

describe('group routing profile editor', () => {
  after(() => {
    domWindow.close()
  })

  test('creates a default-backed draft and updates capability choices', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const changes: string[] = []

    function Fixture() {
      const [value, setValue] = useState('{}')
      return (
        <I18nextProvider i18n={i18n}>
          <GroupRoutingProfileEditor
            groupName='premium'
            groupRoutingRequirements={value}
            summary={matchedSummary}
            onChange={(nextValue) => {
              changes.push(nextValue)
              setValue(nextValue)
            }}
            onViewTargets={() => {}}
          />
        </I18nextProvider>
      )
    }

    await act(async () => root.render(<Fixture />))

    await act(async () => findControl(container, 'Adapt from default').click())

    const enabledProfile = JSON.parse(changes.at(-1) ?? '{}').premium
    assert.equal(enabledProfile.status, 'draft')
    assert.equal(enabledProfile.routing_source, 'default')
    assert.equal(enabledProfile.real_person_mode, 'any')
    assert.deepEqual(enabledProfile.allowed_cost_modes, [])

    await act(async () =>
      findControl(container, 'Must support real person').click()
    )
    await act(async () => findControl(container, 'Per duration').click())

    const updatedProfile = JSON.parse(changes.at(-1) ?? '{}').premium
    assert.equal(updatedProfile.real_person_mode, 'required')
    assert.deepEqual(updatedProfile.allowed_cost_modes, ['per_duration'])
    assert.ok(changes.length >= 3)

    await act(async () => root.unmount())
    container.remove()
  })

  test('prevents the source and auto pseudo groups from adapting from default', async () => {
    for (const groupName of ['default', 'auto']) {
      const container = document.createElement('div')
      document.body.append(container)
      const root = createRoot(container)

      await act(async () => {
        root.render(
          <I18nextProvider i18n={i18n}>
            <GroupRoutingProfileEditor
              groupName={groupName}
              groupRoutingRequirements='{}'
              summary={matchedSummary}
              onChange={() => {}}
              onViewTargets={() => {}}
            />
          </I18nextProvider>
        )
      })

      const adaptSwitch = container.querySelector(
        '[role="switch"][aria-label="Adapt from default"]'
      )
      assert.ok(adaptSwitch instanceof HTMLElement)
      assert.equal(adaptSwitch.hasAttribute('data-disabled'), true)
      const adaptButton = [...container.querySelectorAll('button')].find(
        (button) => button.textContent?.trim() === 'Adapt from default'
      )
      assert.ok(adaptButton instanceof HTMLButtonElement)
      assert.equal(adaptButton.disabled, true)

      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('blocks activation when no compatible targets are matched', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <GroupRoutingProfileEditor
            groupName='default'
            groupRoutingRequirements={JSON.stringify({
              default: {
                status: 'draft',
                routing_source: 'default',
              },
            })}
            summary={{ ...matchedSummary, matched_targets: 0 }}
            onChange={() => {}}
            onViewTargets={() => {}}
          />
        </I18nextProvider>
      )
    })

    const activeButton = findControl(container, 'Active')
    assert.ok(activeButton instanceof HTMLButtonElement)
    assert.equal(activeButton.disabled, true)
    assert.match(container.textContent ?? '', /No compatible targets/)

    await act(async () => root.unmount())
    container.remove()
  })

  test('disabling adaptation preserves a legacy real-person requirement', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    let changedValue = ''

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <GroupRoutingProfileEditor
            groupName='legacy'
            groupRoutingRequirements={JSON.stringify({
              legacy: {
                require_real_person: true,
                status: 'draft',
                routing_source: 'default',
                real_person_mode: 'forbidden',
                allowed_cost_modes: ['free'],
              },
            })}
            summary={matchedSummary}
            onChange={(nextValue) => {
              changedValue = nextValue
            }}
            onViewTargets={() => {}}
          />
        </I18nextProvider>
      )
    })

    await act(async () => findControl(container, 'Adapt from default').click())

    assert.deepEqual(JSON.parse(changedValue), {
      legacy: { require_real_person: true },
    })

    await act(async () => root.unmount())
    container.remove()
  })

  test('explicit real-person modes replace conflicting legacy booleans', async () => {
    const cases = [
      {
        groupName: 'legacy-required',
        legacyValue: true,
        initialMode: 'required',
        nextLabel: 'Must not support real person',
        expectedMode: 'forbidden',
      },
      {
        groupName: 'legacy-any',
        legacyValue: false,
        initialMode: 'any',
        nextLabel: 'Must support real person',
        expectedMode: 'required',
      },
    ] as const

    for (const testCase of cases) {
      const container = document.createElement('div')
      document.body.append(container)
      const root = createRoot(container)
      let changedValue = ''

      await act(async () => {
        root.render(
          <I18nextProvider i18n={i18n}>
            <GroupRoutingProfileEditor
              groupName={testCase.groupName}
              groupRoutingRequirements={JSON.stringify({
                [testCase.groupName]: {
                  require_real_person: testCase.legacyValue,
                  status: 'draft',
                  routing_source: 'default',
                  real_person_mode: testCase.initialMode,
                },
              })}
              summary={matchedSummary}
              onChange={(nextValue) => {
                changedValue = nextValue
              }}
              onViewTargets={() => {}}
            />
          </I18nextProvider>
        )
      })

      await act(async () => findControl(container, testCase.nextLabel).click())

      const changedProfile = JSON.parse(changedValue)[testCase.groupName]
      assert.equal(changedProfile.real_person_mode, testCase.expectedMode)
      assert.equal(Object.hasOwn(changedProfile, 'require_real_person'), false)

      await act(async () => root.unmount())
      container.remove()
    }
  })
})
