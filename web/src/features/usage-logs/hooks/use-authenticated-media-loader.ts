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
