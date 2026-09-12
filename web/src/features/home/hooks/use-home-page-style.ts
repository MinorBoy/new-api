import { useEffect, useState } from 'react'

import { getHomePageStyle } from '../api'

const STORAGE_KEY = 'home_page_style'

export type HomePageStyle = 'default' | 'living-system'

/**
 * Read the cached style from localStorage so the first paint matches the
 * last-known value (avoids a flash of the default composition when the
 * admin has switched to living-system). Falls back to 'default' on any
 * miss / parse error / stale value.
 */
function readCachedStyle(): HomePageStyle {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'living-system') return 'living-system'
  } catch {
    // ignore storage errors (private mode, quota, etc.)
  }
  return 'default'
}

/**
 * Hook to load the admin-configured home page composition.
 *
 * Mirrors useHomePageContent: localStorage cache first for immediate
 * render, then a background fetch of GET /api/home_page_style to pick up
 * admin changes. The hook never throws — on fetch failure it falls back
 * to the cached value (or 'default') and marks itself loaded.
 */
export function useHomePageStyle(): { style: HomePageStyle; loaded: boolean } {
  const [style, setStyle] = useState<HomePageStyle>(readCachedStyle)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let mounted = true

    const load = async () => {
      try {
        const response = await getHomePageStyle()
        if (!mounted) return
        const next: HomePageStyle =
          response.success && response.data === 'living-system'
            ? 'living-system'
            : 'default'
        setStyle(next)
        setLoaded(true)
        try {
          localStorage.setItem(STORAGE_KEY, next)
        } catch {
          // ignore storage write errors
        }
      } catch {
        if (!mounted) return
        // Keep the cached value; just mark loaded so the UI can render.
        setLoaded(true)
      }
    }

    load()

    return () => {
      mounted = false
    }
  }, [])

  return { style, loaded }
}
