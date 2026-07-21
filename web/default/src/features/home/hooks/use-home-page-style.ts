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
