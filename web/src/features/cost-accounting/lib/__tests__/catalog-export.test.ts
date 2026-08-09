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

import { Window } from 'happy-dom'

import {
  downloadCostCatalogExport,
  filenameFromContentDisposition,
} from '../catalog-export'

test('prefers RFC 5987 filenames and removes directory traversal', () => {
  assert.equal(
    filenameFromContentDisposition(
      "attachment; filename=old.csv; filename*=UTF-8''..%2F..%2F%E4%BE%9B%E5%BA%94%E5%95%86%3A%E6%88%90%E6%9C%AC.csv",
      'fallback.csv'
    ),
    '供应商_成本.csv'
  )
  assert.equal(
    filenameFromContentDisposition(
      'attachment; filename="..\\reports\\catalog.csv"',
      'fallback.csv'
    ),
    'catalog.csv'
  )
})

test('downloads once and always releases the Blob URL', () => {
  const browserWindow = new Window({ url: 'http://localhost/' })
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const previousDocument = Object.getOwnPropertyDescriptor(
    globalThis,
    'document'
  )
  const previousURL = Object.getOwnPropertyDescriptor(globalThis, 'URL')
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: browserWindow,
  })
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: browserWindow.document,
  })
  let created = 0
  let revoked = 0
  let clicked = 0
  const urlAPI = {
    createObjectURL: () => {
      created++
      return 'blob:catalog'
    },
    revokeObjectURL: (value: string) => {
      assert.equal(value, 'blob:catalog')
      revoked++
    },
  }
  Object.defineProperty(globalThis, 'URL', {
    configurable: true,
    value: urlAPI,
  })
  const clickDescriptor = Object.getOwnPropertyDescriptor(
    browserWindow.HTMLAnchorElement.prototype,
    'click'
  )
  Object.defineProperty(browserWindow.HTMLAnchorElement.prototype, 'click', {
    configurable: true,
    value: () => {
      clicked++
    },
  })

  try {
    downloadCostCatalogExport({
      blob: new Blob(['catalog']),
      filename: '../unsafe:catalog.csv',
      rowCount: 1,
    })
    assert.equal(created, 1)
    assert.equal(clicked, 1)
    assert.equal(revoked, 1)
  } finally {
    if (clickDescriptor === undefined) {
      Reflect.deleteProperty(browserWindow.HTMLAnchorElement.prototype, 'click')
    } else {
      Object.defineProperty(
        browserWindow.HTMLAnchorElement.prototype,
        'click',
        clickDescriptor
      )
    }
    for (const [key, descriptor] of [
      ['window', previousWindow],
      ['document', previousDocument],
      ['URL', previousURL],
    ] as const) {
      if (descriptor === undefined) {
        delete (globalThis as Record<string, unknown>)[key]
      } else {
        Object.defineProperty(globalThis, key, descriptor)
      }
    }
    browserWindow.close()
  }
})
