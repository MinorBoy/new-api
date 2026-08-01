// @ts-expect-error Bun supplies mock.module at test runtime; the frontend
// typecheck only includes Node's test declarations.
import { mock } from 'bun:test'
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
import { describe, test } from 'node:test'

import { createElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

type MockProps = {
  children?: ReactNode
  placeholder?: string
}

mock.module('@tanstack/react-router', () => ({
  Link: (props: MockProps) => createElement('a', null, props.children),
  useNavigate: () => () => Promise.resolve(),
}))

mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => `translated:${key}` }),
}))

mock.module('@/components/ui/button', () => ({
  Button: (props: MockProps) =>
    createElement('button', { type: 'button' }, props.children),
}))

mock.module('@/components/ui/command', () => ({
  CommandDialog: (props: MockProps) =>
    createElement('div', null, props.children),
  CommandEmpty: (props: MockProps) =>
    createElement('div', null, props.children),
  CommandGroup: (props: MockProps) =>
    createElement('div', null, props.children),
  CommandInput: (props: MockProps) =>
    createElement('input', { placeholder: props.placeholder }),
  CommandItem: (props: MockProps) => createElement('div', null, props.children),
  CommandList: (props: MockProps) => createElement('div', null, props.children),
}))

mock.module('../lib/use-doc-locale', () => ({
  useDocLocale: () => 'en',
}))

const { DocsSearch } = await import('../components/docs-search')
const { DocsSidebar } = await import('../components/docs-sidebar')

describe('documentation navigation', () => {
  test('does not advertise an unimplemented search shortcut', () => {
    const html = renderToStaticMarkup(<DocsSearch />)

    assert.match(html, /translated:Search docs/)
    assert.doesNotMatch(html, /<kbd/)
    assert.doesNotMatch(html, /⌘K/)
  })

  test('localizes the sidebar accessible name', () => {
    const html = renderToStaticMarkup(
      <DocsSidebar activeSlug='overview' locale='en' />
    )

    assert.match(html, /aria-label="translated:Documentation"/)
  })
})
