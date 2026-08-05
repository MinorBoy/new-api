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
import { useCallback, useEffect, useRef } from 'react'

import { api } from '@/lib/api'

export function useAuthenticatedMediaLoader() {
  const objectURLs = useRef(new Set<string>())
  const abortController = useRef<AbortController | null>(null)

  const release = useCallback(() => {
    abortController.current?.abort()
    abortController.current = null
    for (const objectURL of objectURLs.current) {
      URL.revokeObjectURL(objectURL)
    }
    objectURLs.current.clear()
  }, [])

  const load = useCallback(async (proxyPath: string): Promise<string> => {
    abortController.current ??= new AbortController()
    const signal = abortController.current.signal
    const response = await api.get<Blob>(proxyPath, {
      responseType: 'blob',
      skipErrorHandler: true,
      signal,
    })
    if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
    const objectURL = URL.createObjectURL(response.data)
    objectURLs.current.add(objectURL)
    return objectURL
  }, [])

  useEffect(() => release, [release])

  return { load, release }
}
