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
import type { CostCatalogExportResult } from '../types'

function sanitizeFilenameComponent(value: string): string {
  return [...value]
    .map((character) => {
      const code = character.charCodeAt(0)
      return code < 32 || '\\/:*?"<>|'.includes(character) ? '_' : character
    })
    .join('')
}

function unquoteFilename(value: string): string {
  if (value.startsWith('"') && value.endsWith('"')) {
    return value.slice(1, -1)
  }
  return value
}

function safeCatalogFilename(value: string, fallback: string): string {
  const basename = value.split(/[\\/]/).at(-1)?.trim() ?? ''
  const sanitized = sanitizeFilenameComponent(basename)
  if (sanitized && sanitized !== '.' && sanitized !== '..') {
    return sanitized
  }
  const fallbackBasename = fallback.split(/[\\/]/).at(-1)?.trim() ?? ''
  return (
    sanitizeFilenameComponent(fallbackBasename) || 'supplier-cost-catalog.csv'
  )
}

export function filenameFromContentDisposition(
  header: string | undefined,
  fallback: string
): string {
  let candidate = ''
  const encoded = header?.match(/filename\*\s*=\s*(?:UTF-8'')?([^;]+)/i)?.[1]
  if (encoded) {
    const raw = unquoteFilename(encoded.trim())
    try {
      candidate = decodeURIComponent(raw)
    } catch {
      candidate = raw
    }
  }
  if (!candidate) {
    const fallbackMatch = header?.match(
      /(?:^|;)\s*filename\s*=\s*("[^"]*"|[^;]+)/i
    )?.[1]
    candidate = unquoteFilename(fallbackMatch?.trim() ?? '')
  }
  return safeCatalogFilename(candidate, fallback)
}

export function downloadCostCatalogExport(
  result: CostCatalogExportResult,
  fallback = 'supplier-cost-catalog.csv'
): void {
  const objectURL = URL.createObjectURL(result.blob)
  const anchor = document.createElement('a')
  try {
    anchor.href = objectURL
    anchor.download = safeCatalogFilename(result.filename, fallback)
    document.body.append(anchor)
    anchor.click()
  } finally {
    anchor.remove()
    URL.revokeObjectURL(objectURL)
  }
}
