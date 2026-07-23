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
import test from 'node:test'

import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { FormProvider, useForm } from 'react-hook-form'
import { I18nextProvider } from 'react-i18next'

import {
  createEmptyPolicyForm,
  createEmptyTarget,
  type RoutingPolicyFormValues,
} from '../types'
import { RouteTargetEditor } from './route-target-editor'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

function TargetEditorFixture() {
  const policy = createEmptyPolicyForm()
  const form = useForm<RoutingPolicyFormValues>({
    defaultValues: { ...policy, targets: [createEmptyTarget()] },
  })

  return (
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
  )
}

test('target action buttons keep accessible labels, tooltips, and hit areas', () => {
  const html = renderToStaticMarkup(
    createElement(I18nextProvider, { i18n }, createElement(TargetEditorFixture))
  )
  const actionButtons = [
    ...html.matchAll(/<button[^>]*aria-label="(?:Copy|Delete)"[^>]*>/g),
  ]

  assert.equal(actionButtons.length, 2)
  for (const match of actionButtons) {
    assert.match(match[0], /\bsize-9\b/)
    assert.match(match[0], /title="(?:Copy|Delete)"/)
  }
})
